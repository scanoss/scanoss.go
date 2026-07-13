# Implementation Plan: adopt the published scanoss.api Go SDK for model types

**Spec:** `./spec.md` · **Status:** Draft

## Approach
Complete the in-flight swap from the locally-generated `pkg/scanoss/openapi` package to the
external `github.com/scanoss/scanoss.api/sdk/go` module (package `scanossapi`), then delete
the local codegen machinery. The work is a mechanical substitution. For the **service response types** the external SDK's
names are already identical to the local generated names (`VulnerabilitiesResponse`, …) — only
the import path changes. For the **scan-result cluster**, the `scanoss.api` spec renames the
schema keys (Option A) so the SDK exports the **same friendly names** scanoss already uses
(`ScanEnvelope`, `ScanResult`, `ScanServer`, `KnowledgeBase`, `FileResult`, `MatchResult`,
`ComponentResult`, `LineRange`). The domain aliases in `scan_result.go` are **removed
entirely**: every consumer that used the unqualified `ScanResult`/`ScanEnvelope`/… is
rewritten to `scanossapi.ScanResult`/`scanossapi.ScanEnvelope`/…. Because the identifiers are
unchanged, `pkg/scanoss`'s public signatures keep the same type *names* — only their source
package moves (`scanoss.X`/local `openapi.X` → `scanossapi.X`). One vocabulary, no re-export to
keep in step, and no rename churn for downstream code beyond swapping the import.

Order the work so the tree compiles green at each commit: first finish the type swap (which
fixes the currently-broken build), then remove the now-unused codegen inputs and targets.

## Dependency & versioning decision
The SDK is a **Go module submodule**: `scanoss.api` has a nested `go.mod` at `sdk/go` whose
module path is `github.com/scanoss/scanoss.api/sdk/go`. Submodules are versioned with
**path-prefixed tags** (`sdk/go/vX.Y.Z`), independent of the repo's root tags.

**Chosen version: `v0.4.2`.** The `scanoss.api` release `v0.4.2` (commit `7b8bdf2`) already
carries the friendly-name renames and the pointer `server`/`knowledge_base` fix. Caveat: the
release cut only a **bare/root** `v0.4.2` tag, which does **not** version the `sdk/go`
submodule. A companion **`sdk/go/v0.4.2`** tag on the same commit is required before
`go get …/sdk/go@v0.4.2` resolves (tasks.md Task 0). The earlier pseudo-version pin
`v0.0.0-20260702153529-70574dbdd56d` (from unmerged commit `70574dbdd56d`) predates the
renames/pointer fix and must not be used for the durable landing.

**Options considered**
1. **`replace` → local clone** (`replace github.com/scanoss/scanoss.api/sdk/go =>
   ../scanoss.api/sdk/go`). Convenient for co-development, but path-dependent and must **never**
   be committed. Use only in a developer's local `go.mod`, reverted before commit.
2. **Pseudo-version** (current state). Resolvable today, reproducible, but points at an
   unmerged, untagged commit — fragile provenance and easy to forget to bump.
3. **Tagged submodule release** (`sdk/go/v0.1.0`). Stable, greppable, `go get`-friendly, and
   the canonical way to depend on a submodule.

**Recommendation:** depend on a **tagged submodule release** (Option 3). Since no `sdk/go`
tag exists yet, that tag is the prerequisite (spec.md → tasks.md Task 0). Until it is cut, the
**pseudo-version already pinned is acceptable as a transitional pin** to keep this branch
building, but the durable landing commit must reference a real tag. Do **not** commit a
`replace` directive; developers may add one locally for parallel work on both repos.

## File-by-file migration map
All 16 importers of `github.com/scanoss/scanoss.go/pkg/scanoss/openapi` swap the import to
`scanossapi "github.com/scanoss/scanoss.api/sdk/go"` and requalify `openapi.X` → `scanossapi.X`.
Type names are unchanged. In addition, the files that referenced the now-deleted **domain
aliases** (unqualified `ScanResult`, `ScanEnvelope`, …) gain the `scanossapi` import and
requalify those to the external type names.

**Domain-alias hub (delete the aliases):**
- `pkg/scanoss/scan_result.go` — **delete** the entire alias block. Keep only
  `parseScanEnvelope` (retyped to return `scanossapi.ScanEnvelope`) and the
  `scanStateCompleted`/`scanStateFailed` constants; import `scanossapi` and drop `openapi`.
  This is also the commit that fixes the broken build (`env.Result` is now
  `*scanossapi.ScanResult` throughout).

**`pkg/scanoss` scan-flow files (import `scanossapi`, requalify former aliases):**
- `scan.go` — every `ScanEnvelope` → `scanossapi.ScanEnvelope` (return types of
  `scan`/`Status`/`Wait` and locals); `e.ScanId`/`e.Status`/`env.Result` unchanged.
- `bomremove.go` — `ApplyBOMRemove(result *ScanResult, …)` →
  `ApplyBOMRemove(result *scanossapi.ScanResult, …)`; `FileResult`/`ComponentResult` in
  helpers → `scanossapi.FileResult`/`scanossapi.ComponentResult`.

**`pkg/scanoss` service files (requalify import):**
- `copyright.go`, `cryptography.go`, `vulnerabilities.go`, `geoprovenance.go`, `components.go`,
  `licenses.go` — return-type qualifiers on service methods (e.g.
  `*openapi.VulnerabilitiesResponse` → `*scanossapi.VulnerabilitiesResponse`).

**`cmd` files (requalify import):**
- `copyright.go`, `cryptography.go`, `licenses.go`, `geoprovenance.go`, `components.go`,
  `vulnerabilities.go` — thin wrappers whose signatures echo the service return types.
- `scan.go` — the `ScanResult` reference (result rendering) → `scanossapi.ScanResult`.

**`pkg/sbom/scansource` (requalify import):**
- `scansource.go` — `LicensesFrom(*openapi.ComponentsLicenseResponse)`,
  `VulnerabilitiesFrom(*openapi.VulnerabilitiesResponse)`, plus former aliases `ScanResult`/
  `FileResult` → `scanossapi.ScanResult`/`scanossapi.FileResult`; field access
  `m.UrlHash` is unchanged (same field on the external type).
- `scansource_test.go` — same requalification in fixtures.

**`pkg/postprocess` (requalify import):**
- `bomremove.go` — the deprecated wrapper's `*scanoss.ScanResult` param follows whatever
  `ApplyBOMRemove` now takes; since the alias is gone, retype to
  `*scanossapi.ScanResult` (import `scanossapi`).

**Tests referencing former aliases (requalify):**
- `pkg/scanoss/scan_test.go`, `pkg/scanoss/scan_result_test.go`,
  `pkg/scanoss/bomremove_test.go`, `cmd/scan_test.go`, `pkg/sbom/scansource/scansource_test.go`
  — every `ScanEnvelope`/`ScanResult`/`FileResult`/`MatchResult`/`ComponentResult` literal →
  the `scanossapi.*` name. Note `scan_result_test.go:48`'s `e.Result.Server != nil` depends on
  the pointer-typed `server` (see risk 1 / Task 0).

**Codegen machinery to delete (no importers left after the swap):**
- `api/openapi.yaml`, `api/oapi-codegen.yaml` (and the `api/` dir if it becomes empty).
- `pkg/scanoss/openapi/` in full: `types.gen.go`, `doc.go`, `version.go`, `version_test.go`.
  Note `version.go`'s `SpecVersion`/`APIVersion` are **not referenced anywhere outside the
  package** (verified), so nothing depends on them — safe to drop. The unrelated
  `cdx.SpecVersion1_7` in `pkg/sbom` is a different symbol and untouched.
- `Makefile` targets `sync-spec`, `generate`, `regenerate` and the vars `OAPI_CODEGEN_VERSION`,
  `SPEC_REPO`, `SPEC_PATH`, `SPEC_REF`. No `//go:generate` directives exist to remove; the
  prose mention lives inside `doc.go`/`oapi-codegen.yaml`, which are deleted anyway.
- Drop the `github.com/oapi-codegen/runtime` **direct** requirement from this repo's `go.mod`
  only if it becomes unused — the external SDK still imports it transitively, so it will
  remain in `go.sum` as an indirect dep; let `go mod tidy` settle the `require` block.

## Relationship to the in-flight local-alias work
This branch's uncommitted changes are a **partial, hand-done** version of this same migration
(they already added the external dependency and pointed `ScanEnvelope` at it) but left the
tree broken. This plan **supersedes** that half-step: rather than finishing a *local*-alias
model (the `git diff` on `scan_result.go` was retargeting aliases to the **local** generated
types), it **deletes the aliases** and points every consumer straight at the **external**
package, then removes the local generator.

The plan's correctness hinges on the published SDK matching the field ergonomics the in-flight
`docs/batch-report-named-schemas` spec establishes:
- **value scalars** via `x-go-type-skip-optional-pointer` — `ScanId string`, `Phase string`,
  progress counters `int`, etc. (the published SDK **already** has these — verified).
- **pointer `server`/`knowledge_base`** so absent nested objects are omitted — the published
  pseudo-version does **not** yet have these (they are value+`omitempty`). Landing must use an
  SDK build that includes them (Task 0), otherwise the fallback below applies.

## Risks & mitigations
1. **Pointer-vs-value `server`/`knowledge_base` (highest risk).** Published SDK exposes them as
   values; the local generated types and `scan_result_test.go:48` (`e.Result.Server != nil`)
   assume pointers. *Mitigation:* land against an SDK build that keeps them pointers (Task 0
   prerequisite). *Fallback* if that build is unavailable: change the test's emptiness check
   from `!= nil` to a zero-value comparison and accept that an absent server block serializes
   as an empty object — document as a known regression and a follow-up on `scanoss.api`.
2. **Untagged/unmerged pin.** The current pseudo-version is from an unmerged branch. *Mitigation:*
   treat the tagged `sdk/go` release as the prerequisite; keep the pseudo-version only as a
   transitional pin and bump to the tag in the same series.
3. **Initialism / field-name drift** (e.g. `UrlHash` vs `URLHash`, `ScanId` vs `ScanID`,
   `ApiVersion`). *Mitigation:* none needed for the swap itself — the external names match the
   local generated names exactly (both oapi-codegen), and the working tree already uses the
   generated spellings (`m.UrlHash`, `e.ScanId`). The only `URLHash`/`.URL` hits in the repo
   are on the **neutral** `sbom.Component` type (`pkg/sbom/spdxlite.go`), unrelated to the
   openapi types.
4. **`Status` enum type.** External `ScanEnvelope.Status` is a typed string enum (named
   `ScanEnvelopeStatus` after the schema rename); the local `scanStateCompleted`/
   `scanStateFailed` constants and `parseScanEnvelope`'s `e.Status == ""` guard are
   untyped-string comparisons that still compile against the enum. *Mitigation:* none needed;
   keep the constants and the guard as-is.
5. **Test fixtures.** `pkg/scanoss/testdata/scan_envelope_{file,none,snippet}.json` are wire
   JSON and unmarshal into the external types unchanged; `scan_result_test.go` exercises them.
   *Mitigation:* re-run these tests after the swap; the only behavioral coupling is risk (1).
6. **Deleting `version_test.go`.** It reads `api/openapi.yaml`; once the spec is gone the test
   is meaningless. *Mitigation:* it is removed as part of deleting the `openapi` package — no
   drift guard is lost because the spec no longer lives here (the contract owner guards it).

## Mechanics (commit-sized steps)
1. **Pin the SDK** — ensure `go.mod` requires the chosen SDK version (tag preferred; the
   already-pinned pseudo-version otherwise) and `go mod tidy` moves it to a direct `require`.
2. **Delete the domain aliases** in `scan_result.go` (keep `parseScanEnvelope` + the state
   constants), and requalify the in-package scan-flow users (`scan.go`, `bomremove.go`) to
   `scanossapi.*` in the same commit so `pkg/scanoss` builds green.
3. **Requalify remaining importers** (`pkg/scanoss/*` service files, `cmd/*`,
   `pkg/sbom/scansource/*`, `pkg/postprocess/*`, and the affected tests) to `scanossapi` — one
   commit (mechanical, no behavior change).
4. **Delete the codegen inputs** — `api/openapi.yaml`, `api/oapi-codegen.yaml`, and the
   `pkg/scanoss/openapi/` package.
5. **Delete the Makefile targets/vars** — `sync-spec`, `generate`, `regenerate`, and the spec
   vars.
6. **Verify** — `make check` (fmt-check + vet + lint + test), plus a manual `go build ./...`.
