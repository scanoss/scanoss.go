# Tasks: typed API contracts from OpenAPI

**Spec:** [`spec.md`](./spec.md) · **Plan:** [`plan.md`](./plan.md)

`[P]` = can run in parallel (different files). Paths are relative to the repo root.

## Conventions
- **Conventional Commits**; **atomic commits** (review before each); **no AI refs**;
  **short** imperative subjects. Every code change ships with tests.

## Phase 0 — Foundation (implement first)
- [ ] **T001** `api/openapi.yaml` + `api/oapi-codegen.yaml`; `make generate` target.
- [ ] **T002** Generate `pkg/scanoss/openapi` (public, types-only).
- [ ] **T003** `As[T]` decode helper in `decorate.go`.
- [ ] **T004** `cmd`: `runPurlServiceTyped[T]` + `newPurlServiceCmdTyped[T]`.
- [ ] **T005** Type the `vulnerabilities` service + CLI; repoint engine tests to
      `decorate`/`decorateOne`.

## Phase 1 — Versioning
- [ ] **T010** `pkg/scanoss/openapi/version.go`: `SpecVersion` (== `info.version`) and
      `APIVersion = "v3"` constants. Optionally have `make generate` extract
      `info.version` and write the constant to avoid manual drift.
- [ ] **T011** `.github/workflows/ci.yml`: `spec-drift` job (git-only, PR-gated) that
      fails if `api/openapi.yaml` and `pkg/scanoss/openapi/types.gen.go` did not change
      together against the base branch.
- [ ] **T012** (optional) `Makefile`: local `generate-check` target (`make generate` +
      `git diff --exit-code pkg/scanoss/openapi`) for a stronger, regenerate-and-compare
      check; not in the CI gate since it downloads oapi-codegen.
- [ ] **T013** Test: read `api/openapi.yaml` `info.version` and assert it equals
      `openapi.SpecVersion` (guards the constant against drift).
- [ ] **T014** (maintainer prerequisite) Create a GitHub App (owner `scanoss`,
      Repository permission `Contents: Read-only`), **install it on `scanoss.api`**, and
      add its App ID + private key as the `scanoss` Actions secrets `SCANOSS_APP_ID` /
      `SCANOSS_APP_PRIVATE_KEY`. Cannot be automated; required before T015 can run.
- [x] **T015** `.github/workflows/spec-upstream.yml`: PR-triggered (+ `workflow_dispatch`)
      job that mints a repo-scoped token from the GitHub App
      (`actions/create-github-app-token`), fetches `scanoss.api@main:docs/openapi.yaml`
      via `gh api`, and **fails** on any `diff` vs `api/openapi.yaml`. Track `main` (the
      latest tag lags). On failure: re-sync spec → `make generate` → commit.

## Phase 2 — Type the remaining services (one commit each)
- [ ] **T020 [P]** `cryptography.go` + `cmd/cryptography.go`: typed returns
      (`CryptoAlgorithmsResponse`, `CryptoAlgorithmsInRangeResponse`,
      `CryptoVersionsInRangeResponse`, `CryptoHintsResponse`, `CryptoHintsInRangeResponse`).
- [ ] **T021 [P]** `licenses.go` + `cmd/licenses.go`: `AttributionResponse`,
      `LicenseEvidenceResponse`.
- [ ] **T022 [P]** `geoprovenance.go` + `cmd/geoprovenance.go`: `GeoOriginResponse`,
      `GeoContributorsResponse`.
- [ ] **T023 [P]** `copyright.go` + `cmd/copyright.go`: `CopyrightEvidenceResponse`,
      `CopyrightHoldersResponse`.
- [ ] **T024** `components.go` + `cmd/components.go`: `ComponentsSearchResponse`,
      `ComponentVersionsResponse`, `ComponentsStatusResponse` (search/versions are GET).
- [ ] **T025** Per service: stub-server typed test + live test gated on `SCANOSS_API_KEY`.

## Phase 3 — Decisions (resolve before/at rollout)
- [ ] **T030** Partial-failure handling in the typed path (spec Open decision 1):
      implement the chosen option (recommend fail-closed on any chunk error).
- [ ] **T031** (optional) Prune the spec to supported endpoints to shrink the public
      type surface (spec Open decision 2).

## Phase 4 — Docs
- [ ] **T040** `CHANGELOG.md`: typed service responses + `pkg/scanoss/openapi` +
      `make generate`.
- [ ] **T041** README: a short note on the generated types and `make generate`.

## Final verification
- [ ] `make generate-check` (drift guard) green.
- [ ] `go build ./...`; `go vet ./...`; `gofmt -l`; `go test ./...` (+ live with key).
- [ ] `go doc ./pkg/scanoss/openapi` shows the models; `SpecVersion`/`APIVersion` present.
