# Feature Specification: adopt the published scanoss.api Go SDK for model types

**Feature branch:** `feat/scan-result-restructure` (this branch) · **Status:** Draft

## Summary
Stop generating the OpenAPI model types **locally** and depend on the **published** external
Go SDK instead:

```go
scanossapi "github.com/scanoss/scanoss.api/sdk/go"
```

Today the types come from a local codegen pipeline: `make sync-spec` pulls
`docs/openapi.yaml` from `scanoss/scanoss.api@main` into `api/openapi.yaml`, and
`make generate` runs oapi-codegen (config `api/oapi-codegen.yaml`) to write
`pkg/scanoss/openapi/types.gen.go`. The spec (`api/openapi.yaml`) and the generated package
(`pkg/scanoss/openapi/`) are then committed into this repo. That makes `scanoss` carry a
copy of a contract it does not own and keeps a codegen toolchain (oapi-codegen, `gh` spec
pull) it must run and keep in step.

Instead, the contract owner (`scanoss.api`) publishes the generated types as a Go module
submodule (`sdk/go`, package `scanossapi`). `scanoss` consumes that module like any other
dependency: no committed spec, no committed generated code, no codegen targets. Every
consumer that imports `github.com/scanoss/scanoss.go/pkg/scanoss/openapi` is rewired to the
external package, and the local codegen machinery is deleted.

The domain aliases in `pkg/scanoss/scan_result.go` (`ScanEnvelope`, `ScanResult`,
`ScanServer`, `KnowledgeBase`, `FileResult`, `MatchResult`, `LineRange`, `ComponentResult`)
are **removed**. Consumers use the external `scanossapi.*` types **directly** — and those
types **carry the friendly names**, because the `scanoss.api` OpenAPI spec renames the
batch-report schema keys to scanoss's vocabulary (see "External-package prerequisite").
So the SDK exports `scanossapi.ScanEnvelope`, `scanossapi.ScanResult`, `scanossapi.ScanServer`,
`scanossapi.KnowledgeBase`, `scanossapi.FileResult`, `scanossapi.MatchResult`,
`scanossapi.LineRange`, `scanossapi.ComponentResult` — no re-export layer and no ugly codegen
names. `pkg/scanoss`'s exported signatures (e.g. `Scan.Folder`/`Files`/`WFP`/`Status`/`Wait`
returning `scanossapi.ScanEnvelope`, and `ApplyBOMRemove(*scanossapi.ScanResult, …)`) speak
those names directly. The **identifiers do not change** — only their source package moves
from `scanoss`/local `openapi` to `scanossapi`, so downstream churn is limited to the import
path/qualifier, not the names.

## Current state (why this is needed)
- `api/openapi.yaml` — a committed copy of a spec owned by `scanoss/scanoss.api`.
- `api/oapi-codegen.yaml` — oapi-codegen config (`package: openapi`, models only).
- `pkg/scanoss/openapi/` — the generated package: `types.gen.go` (~112 KB, generated),
  `doc.go` (package doc pointing at `make generate`), `version.go`
  (`SpecVersion`/`APIVersion` constants), `version_test.go` (guards `SpecVersion` against
  `api/openapi.yaml`'s `info.version`).
- `Makefile` — targets `sync-spec` (gh pull), `generate` (oapi-codegen), `regenerate`
  (`sync-spec` + `generate`), plus vars `OAPI_CODEGEN_VERSION`, `SPEC_REPO`, `SPEC_PATH`,
  `SPEC_REF`. No target depends on `generate`; there are **no** `//go:generate` directives
  (only a prose mention in `doc.go` / `oapi-codegen.yaml`).
- **16 Go files** import the local package (see plan.md for the full map). Consumers use two
  disjoint groups of types:
  - **scan-result domain** (`BatchReport`, `BatchReportServer`, `BatchReportKnowledgeBase`,
    `BatchReportFile`, `BatchReportMatch`, `LineRange`, `BatchReportComponent`,
    `StatusSnapshot`) — re-exported as domain aliases in `scan_result.go`.
  - **service response types** (`VulnerabilitiesResponse`, `GeoOriginResponse`,
    `CryptoAlgorithmsResponse`, `ComponentsLicenseResponse`, `LicenseInfo`, `Vulnerability`,
    … ~25 distinct types) — used directly by `pkg/scanoss/*.go` and `cmd/*.go`.
- **In-flight, broken working tree.** This branch already began the migration by hand and is
  **mid-flight**: `pkg/scanoss/scan_result.go` now aliases `ScanEnvelope =
  scanossapi.StatusSnapshot` (external) while the remaining aliases still point at the local
  `openapi.*` types, and `go.mod`/`go.sum` already pin the external SDK. As a result the tree
  **does not compile** (`scan.go:161`: `*scanossapi.BatchReport` vs local `*ScanResult`).
  This feature supersedes and completes that partial local-alias work: it finishes the swap
  to the external package everywhere and removes the local codegen.

## Scope
- Add/finalize the `github.com/scanoss/scanoss.api/sdk/go` dependency in `go.mod`/`go.sum`
  and decide how it is versioned/pinned (see plan.md).
- Delete the domain aliases in `pkg/scanoss/scan_result.go` (keep `parseScanEnvelope` and the
  scan-state constants) and drop the local `openapi` import there.
- Rewire every importer — the 15 other files plus the in-package scan-flow/test files that
  used the aliases — to reference `scanossapi.*` types directly.
- Delete the local codegen machinery: `api/openapi.yaml`, `api/oapi-codegen.yaml`, the whole
  `pkg/scanoss/openapi/` package, and the Makefile targets `sync-spec`/`generate`/`regenerate`
  with their vars.
- Keep the tree building green and tests passing at every step; final `make check`.

## Non-goals
- Renaming or re-wrapping the SDK types. The aliases are removed, not renamed; consumers use
  the `scanossapi.*` names as-is (no new abstraction layer replaces the deleted aliases).
- Consuming the SDK's client/server code — only its **model types** are used (as today).
- Changing wire behavior or the scan-result JSON shape. Field ergonomics are governed by the
  published SDK's spec extensions, not by this repo (see the prerequisite below).
- Owning or editing the spec. `api/openapi.yaml` leaves this repo entirely; the contract
  lives in `scanoss.api`.

## External-package prerequisite (hard dependency)
The migration cannot land cleanly until the published SDK is fit to depend on. As of this
writing (2026-07-02):

- **The module exists and resolves.** `github.com/scanoss/scanoss.api/sdk/go` is a real Go
  submodule (own `go.mod`, package `scanossapi`) and is already fetched into this repo's
  `go.sum` at pseudo-version `v0.0.0-20260702153529-70574dbdd56d`. It exports **every** type
  the consumers use (all ~27 verified present).
- **But it is not release-ready:**
  1. The pinned commit `70574dbdd56d` comes from the **unmerged** `feat/openapi-go-types-sdk`
     branch — it is **not on `scanoss.api@main`**, and the `sdk/go` submodule has **no semver
     tag** (only root tags `v0.2.0`…`v0.4.1` exist, none for `sdk/go`).
  2. **Field-ergonomics divergence.** The published SDK generates `BatchReport.Server` and
     `BatchReportServer.KnowledgeBase` as **value** types with `omitempty`. `omitempty` cannot
     omit a zero struct, so an absent server block would serialize as `"server":{...zeros...}`.
     The in-flight `docs/batch-report-named-schemas` spec work makes those two fields
     **pointers** on purpose so they are omitted when absent — and `scanoss`'s test
     `pkg/scanoss/scan_result_test.go:48` (`e.Result.Server != nil`) depends on that pointer
     semantics. Migrating to the current published pseudo-version regresses this ergonomic and
     breaks that test.

**Prerequisite:** before this migration lands on a durable pin, `scanoss.api` must publish an
`sdk/go` build that (a) is merged to `main`, (b) is tagged with a submodule semver tag
(`sdk/go/vX.Y.Z`), (c) bakes in the `docs/batch-report-named-schemas` spec extensions —
`x-go-type-skip-optional-pointer` for value-able scalars (so `scan_id` is a `string`, progress
counters are `int`, etc.) **and** pointer types for `server`/`knowledge_base` so absent nested
objects are omitted — and (d) **renames the batch-report schema keys** to scanoss's friendly
vocabulary (**Option A**), so the generated Go types are named `ScanEnvelope`, `ScanResult`,
`ScanServer`, `KnowledgeBase`, `FileResult`, `MatchResult`, `ComponentResult` (`LineRange`
unchanged) instead of `StatusSnapshot`/`BatchReport*`. The rename is done on the schema keys
themselves (and every `$ref`), so it also propagates to the spec's docs and to any other
generator/consumer of `scanoss.api` — a wider blast radius than the Go-only `x-go-name`
alternative, and it may touch `scanoss.api`'s own server/CLI code that references the old
generated names. Because the schema *title* is not on the wire (only the `json` property tags
are), (d) is **not** a runtime/API break; it is a naming/codegen change. If (c) is not met,
`scanoss` must instead adapt its consumer code and tests to value-typed
`server`/`knowledge_base` (a documented fallback in plan.md, less desirable). This dependency
is a **blocker** for a clean, tagged landing and is called out as Task 0 in tasks.md.
