# Implementation Plan: typed API contracts from OpenAPI

**Spec:** [`spec.md`](./spec.md) · **Tasks:** [`tasks.md`](./tasks.md)

## Technical context
- **Language/module:** Go, `github.com/scanoss/scanoss.go`.
- **Generator:** `oapi-codegen` v2 (types-only), pinned in the Makefile. No runtime dep.
- **Source of truth:** `api/openapi.yaml` (SCANOSS Services API v3), committed in-repo.
- **Reused, unchanged:** the chunk/merge engine (`decorate`/`decorateOne`/`do`), the
  `Service` vars, `*Result`, `As[T]`.

## Design

### Generation
```yaml
# api/oapi-codegen.yaml
package: openapi
output: pkg/scanoss/openapi/types.gen.go
generate:
  models: true            # types only
output-options:
  skip-prune: true
```
```bash
make generate   # go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@<pinned> -config api/oapi-codegen.yaml api/openapi.yaml
```
Output is a public package `pkg/scanoss/openapi` so services *and* the CLL can name the
types directly (no alias indirection). The generated file is never hand-edited.

### Typed services (the pattern)
```go
import "github.com/scanoss/scanoss.go/pkg/scanoss/openapi"

type VulnerabilityAPI interface {
    Components(ctx context.Context, comps []Component) (*openapi.VulnerabilitiesResponse, error)
    // …
}
func (s vulnerabilityService) Components(ctx context.Context, comps []Component) (*openapi.VulnerabilitiesResponse, error) {
    res, err := s.c.decorate(ctx, ServiceVulnerabilities, comps)
    if err != nil { return nil, err }
    return As[openapi.VulnerabilitiesResponse](res)
}
```
`As[T]` (in `decorate.go`) does merge + `Unmarshal`. Single (`GET`) methods are the same
over `decorateOne`. Roll the pattern out to all six services using the existing
`TODO(typed)` method→model mapping.

### CLI
`runPurlServiceTyped[T]` (in `cmd/purlcommon.go`) resolves components, runs the typed
method, and `json.MarshalIndent`s the model to the `--output`/stdout. `newPurlServiceCmdTyped[T]`
builds the subcommands. Progress unchanged.

### Versioning mechanics
- **Pin = git commit.** `make generate` always regenerates from the committed spec;
  types ⇔ spec are in sync at every commit.
- **Version constants.** Add `pkg/scanoss/openapi/version.go` (a tiny, non-generated
  file, or a value injected by `make generate`) exposing:
  ```go
  const SpecVersion = "0.1.0" // == api/openapi.yaml info.version
  const APIVersion  = "v3"
  ```
  Keep `SpecVersion` in step with `info.version` (the bump policy below; optionally a
  `make` step greps `info.version` and writes the constant to avoid manual drift).
- **Drift gate (CI, git-only).** A `spec-drift` job in `.github/workflows/ci.yml`,
  running on pull requests, compares the PR against the base branch and fails if the
  spec and the generated types did not change together:
  ```bash
  base="origin/${{ github.base_ref }}"
  spec=$(git diff --name-only "$base"...HEAD -- api/openapi.yaml)
  types=$(git diff --name-only "$base"...HEAD -- pkg/scanoss/openapi/types.gen.go)
  # fail if exactly one of {spec, types} changed
  ```
  No extra tooling installed. It guarantees the two move together; it does not
  re-derive the types from the spec. For that stronger check, an optional
  `make generate-check` (`make generate` + `git diff --exit-code pkg/scanoss/openapi`)
  can be run locally — it downloads oapi-codegen, so it is not in the minimal CI gate.
- **Upstream spec check (CI, PR-triggered).**
  `api/openapi.yaml` is a synced copy of the canonical `scanoss.api` →
  `docs/openapi.yaml`. A `spec-upstream` workflow (`on: pull_request` +
  `workflow_dispatch`) fetches that file at **`main`** and fails on any content
  difference:
  ```yaml
  # .github/workflows/spec-upstream.yml
  - id: app-token                                  # mint a short-lived, repo-scoped token
    uses: actions/create-github-app-token@v1
    with:
      app-id: ${{ secrets.SCANOSS_APP_ID }}
      private-key: ${{ secrets.SCANOSS_APP_PRIVATE_KEY }}
      owner: scanoss
      repositories: scanoss.api
  - env: { GH_TOKEN: ${{ steps.app-token.outputs.token }} }
    run: gh api repos/scanoss/scanoss.api/contents/docs/openapi.yaml?ref=main
           -H "Accept: application/vnd.github.raw" > /tmp/upstream.yaml
  - run: |
      diff -u /tmp/upstream.yaml api/openapi.yaml || {
        echo "::error::api/openapi.yaml is out of date vs scanoss.api@main. Re-sync, 'make generate', commit."; exit 1; }
  ```
  Track **`main`** (the latest tag `v0.3.0` lags behind it). `scanoss.api` is private
  and the default `GITHUB_TOKEN` cannot read it, so the job reads it via a **GitHub
  App** (Contents:Read, installed on `scanoss.api`) — a maintainer prerequisite: the
  App's `SCANOSS_APP_ID` / `SCANOSS_APP_PRIVATE_KEY` are stored as secrets and the App
  is installed on `scanoss.api`. Minimal tooling: `gh api` + `diff`. Tradeoff: PR-only
  means drift surfaces only while a PR is open (no scheduled run); the
  `workflow_dispatch` trigger allows an ad-hoc check between PRs.
- **Bump policy.** When `scanoss.api` changes the contract: re-sync `api/openapi.yaml`
  from upstream `main`, `make generate`, commit spec + generated types (+ version
  constant) together.
- **(Optional) runtime check.** Compare `openapi.SpecVersion` against `/v3/health` at
  client init and warn on mismatch.

## Files to modify / add
| File | Change |
|---|---|
| `api/openapi.yaml`, `api/oapi-codegen.yaml` | **add**: spec + generator config |
| `pkg/scanoss/openapi/types.gen.go` | **generated**: models (via `make generate`) |
| `pkg/scanoss/openapi/version.go` | **new**: `SpecVersion`/`APIVersion` constants |
| `pkg/scanoss/decorate.go` | **add**: `As[T]` helper |
| `pkg/scanoss/{vulnerabilities,cryptography,licenses,geoprovenance,copyright,components}.go` | typed returns per `TODO(typed)` |
| `cmd/purlcommon.go` | **add**: `runPurlServiceTyped[T]` + `newPurlServiceCmdTyped[T]` |
| `cmd/{…}.go` | typed closures per command |
| `Makefile` | **add**: `generate`; optional local `generate-check` |
| `.github/workflows/ci.yml` | **add**: `spec-drift` + `generate-check` jobs (PR gates) |
| `.github/workflows/spec-upstream.yml` | **new**: PR-triggered (+ `workflow_dispatch`) check that `api/openapi.yaml` == `scanoss.api@main`, via a GitHub App token |
| GitHub App + secrets | **new (maintainer)**: App with Contents:Read installed on `scanoss.api`; `SCANOSS_APP_ID` / `SCANOSS_APP_PRIVATE_KEY` secrets in `scanoss` |
| `CHANGELOG.md` | note typed responses + the openapi package |

## Testing strategy
- **Per service:** a stub-server test asserting the typed method merges chunks and
  populates the generated model's fields.
- **Live (gated on `SCANOSS_API_KEY`):** decode a real v3 response per service.
- **Drift guard:** `make generate-check` green in CI.
- **Version:** assert `openapi.SpecVersion == info.version` (a test that reads the YAML
  and compares), so the constant can't silently drift.
- **Upstream check:** local dry run (uses your `gh` auth) — should print "in sync"
  today, since ours is byte-identical to `scanoss.api@main`:
  ```bash
  gh api repos/scanoss/scanoss.api/contents/docs/openapi.yaml?ref=main \
    -H "Accept: application/vnd.github.raw" > /tmp/up.yaml
  diff -u /tmp/up.yaml api/openapi.yaml && echo "in sync" || echo "drift"
  ```
  After the secret is added, open a PR and confirm the `upstream-spec` check is green.
- `go build ./...`, `go vet`, `gofmt -l`, `go test ./...`.

## Alternatives considered
- **internal/openapi + alias re-exports** — keeps the public surface curated, but adds
  alias indirection (`typed.go`). Rejected in favour of the public package for
  directness; surface can be pruned instead.
- **Generate into `package scanoss`** — would collide with existing names (`Component`,
  `Status`, `Request`). Rejected.
- **Hand-written structs** — drifts from the spec; rejected in favour of generation.

## Risks & rollout
- **Breaking (pre-1.0, SDK):** service methods change return type `*Result` →
  `*openapi.XResponse`. In-repo callers (CLI, tests) updated in the same change; engine
  tests repoint to `decorate`/`decorateOne` directly.
- **Public surface:** ~140 generated types exposed; prune the spec later if desired.
- Roll out one service per commit (atomic), tests alongside.

## Engineering conventions
- **Conventional Commits**; **atomic commits** (review before each); **no AI/assistant
  references**; **short** imperative subjects. Every change ships with tests.
