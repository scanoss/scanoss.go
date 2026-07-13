# Feature Specification: typed API contracts from OpenAPI

**Feature branch:** `spike/typed-api-contracts` (to land on a feature branch)
**Status:** Draft

## Summary
Today the SDK's service methods return a raw `*Result` (merged `json.RawMessage`) and
callers decode it themselves (the `TODO(typed)` markers on each interface method). This
feature makes the SCANOSS Services API **v3 OpenAPI spec the single source of truth for
the request/response contracts**, generates Go types from it, and has **every** service
method return the typed model generated from the spec — so each service's contract is
enforced by the compiler and pinned to a known API version.

## Approach
- `api/openapi.yaml` committed in-repo = source of truth.
- **`oapi-codegen --generate types`** (types only, no client/server) → a public package
  `pkg/scanoss/openapi` (`types.gen.go`). Regenerated via `make generate`.
- **Every** service method (across all six services) returns the generated model for
  its endpoint, decoding the engine's `*Result`. The pattern, shown for one method and
  applied identically to the rest:
  ```go
  func (s vulnerabilityService) Components(ctx, comps) (*openapi.VulnerabilitiesResponse, error) {
      res, err := s.c.decorate(ctx, ServiceVulnerabilities, comps)
      if err != nil { return nil, err }
      return As[openapi.VulnerabilitiesResponse](res)   // merge + Unmarshal
  }
  ```
- The chunk/merge engine (`decorate`/`decorateOne`/`do`) and the `Service` vars are
  unchanged; `As[T]` is the typed-decode glue over `*Result`.

## Versioning the types (pin to an API version)
Three distinct versions, not to be conflated:
- **Path version** `/v3/...` — the API major, lives in the `Service` endpoint strings.
- **Spec/contract version** — `info.version` of `api/openapi.yaml`. **Unreliable:** it
  is stuck at `"0.1.0"` upstream and is not bumped per API change, so it MUST NOT be
  used to detect drift; compare spec **content** instead.
- **SDK module version** — the Go module tag (e.g. `v0.8.0`).

### Upstream source of truth
`api/openapi.yaml` is a **synced copy** of the canonical spec maintained in the private
repo `scanoss/scanoss.api` at `docs/openapi.yaml`. As of this writing our copy is
byte-identical to `scanoss.api@main`. The latest **tag** (`v0.3.0`) is **behind main**
(older route names, fewer endpoints), so the tracked ref is **`main`**, not the tag.

How the types are pinned to the API version:
- **FR-V1** The committed `api/openapi.yaml` is the pin: `make generate` regenerates
  the types from *that* file, so at any git commit the types and the spec are in sync by
  construction (the commit is the lock).
- **FR-V2** The generated package MUST expose the contract version as constants —
  `openapi.SpecVersion` (from `info.version`) and `openapi.APIVersion` (`"v3"`) — so the
  SDK can log/expose it and consumers can assert against it.
- **FR-V3** CI MUST run a **drift gate on pull requests** using only `git` (no extra
  tooling): compare the PR against the base branch and fail if `api/openapi.yaml` and
  `pkg/scanoss/openapi/types.gen.go` did not change together — spec changed without
  regenerated types, or the generated file hand-edited without a spec change. This is
  what keeps the types locked to the spec. (A stronger local check that actually
  regenerates and diffs, `make generate-check`, is optional since it downloads
  oapi-codegen.)
- **FR-V4** Bump policy: an API change updates `info.version` in the spec, runs
  `make generate`, and commits spec + generated types **together** in one change.
- **FR-V5 (optional)** A defensive runtime check MAY compare `openapi.SpecVersion`
  against the API `/v3/health` (or a version header) and warn on mismatch.
- **FR-V6** CI MUST run an **upstream spec check** on **pull requests**: the job
  fetches `scanoss.api@main:docs/openapi.yaml` and **fails** if it differs from our
  committed `api/openapi.yaml`. This catches the API contract moving ahead of the SDK
  and blocks merge until the spec is re-synced. Constraints: track **`main`** (the
  latest tag lags); compare **content** via `diff` (not `info.version`); minimal
  tooling (`gh api` + `diff`). Tradeoff of PR-only: drift surfaces only when a PR is
  open (no scheduled run). Because `scanoss.api` is **private** and the default
  `GITHUB_TOKEN` cannot read another private repo, the job reads it via a **GitHub
  App** (Contents:Read, installed on `scanoss.api`): it mints a short-lived,
  repo-scoped token at runtime with `actions/create-github-app-token`, from the
  `SCANOSS_APP_ID` / `SCANOSS_APP_PRIVATE_KEY` secrets. On failure, the runbook is:
  re-sync `api/openapi.yaml` from upstream, `make generate`, commit spec + regenerated
  types. The workflow also accepts a manual `workflow_dispatch` for ad-hoc checks.

## Requirements

### Functional
- **FR-001** `api/openapi.yaml` is the source of truth, committed in-repo.
- **FR-002** Types are generated (oapi-codegen, types-only) into `pkg/scanoss/openapi`;
  the generated file is never hand-edited.
- **FR-003** **Every** method of **every** decoration service (`vulnerabilities`,
  `cryptography`, `licenses`, `geoprovenance`, `copyright`, `components`) MUST return
  the OpenAPI model generated from the spec for its endpoint (per the `TODO(typed)`
  markers), decoding via `As[T]` over the unchanged engine. No service may keep a raw
  `*Result` return on its public method.
- **FR-004** Inputs: the batch input stays `[]Component` (our request contract, maps to
  the spec's `BatchRequest`); `components search` keeps `ComponentSearch`. Generating
  request/param types is allowed but not required to replace these.
- **FR-005** The CLI keeps working: a generic `runPurlServiceTyped[T]` marshals the
  typed model to JSON for output; progress unchanged.
- **FR-006** The low-level engine stays internal; `*Result` + `As[T]` remain available
  for callers that want raw access.
- Versioning: **FR-V1..FR-V6** above.

### Non-functional
- **NFR-001** No new runtime dependencies (the generated package imports stdlib only).
- **NFR-002** Regeneration is reproducible (oapi-codegen version pinned in the Makefile).

## Scope — services to type
All six decoration services derive their contract from the generated types:
`vulnerabilities`, `cryptography`, `licenses`, `geoprovenance`, `copyright`,
`components`. The method → response-model mapping is the `TODO(typed)` table already
in the service files (e.g. `Vulnerabilities.Components` → `VulnerabilitiesResponse`,
`Cryptography.Algorithms` → `CryptoAlgorithmsResponse`, `Licenses.Attribution` →
`AttributionResponse`, `Geoprovenance.Origins` → `GeoOriginResponse`,
`Copyright.Holders` → `CopyrightHoldersResponse`, `Components.Search` →
`ComponentsSearchResponse`, …).

## Open decisions (resolve during review)
1. **Partial-failure handling in the typed path.** `As[T]` decodes the merged
   successful chunks; per-chunk failures (today surfaced via `*Result.Failed`) are not
   visible on the typed return. Options: (a) typed method returns an error if any chunk
   failed (fail-closed); (b) keep best-effort merge and expose failures via a separate
   accessor; (c) a small `Response[T]` wrapper carrying data + failures. Recommend (a)
   for a clean contract, with raw `*Result` available for best-effort callers.
2. **Public surface pruning.** `pkg/scanoss/openapi` currently exposes ~140 types
   (including endpoints we don't implement: dependencies, reachability, *-contents).
   Option to prune the spec to supported endpoints before generating.
3. **Typed inputs.** Whether to also generate/use request/param types (`BatchRequest`,
   `SearchComponentsParams`) instead of the hand-written `Component`/`ComponentSearch`.

## Out of scope
- gRPC / a separate spec repo (Go-only, in-repo for now).
- Changing the chunk/merge engine.
