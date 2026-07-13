# Implementation Plan: per-service CLI subcommands

**Spec:** [`spec.md`](./spec.md) · **Tasks:** [`tasks.md`](./tasks.md)

## Technical context
- **Language/module:** Go, `github.com/scanoss/scanoss.go`, CLI in `cmd/` (cobra).
- **Reused plumbing:** `addPurlServiceFlags`, `resolveComponents`, `runPurlService`,
  `writeOutput`, `decorateFunc` (all in `cmd/purlcommon.go`) — unchanged.
- **SDK:** the grouped per-service handles (`client.Vulnerabilities.Components`, …)
  already exist; this is a CLI-only change.
- **No new dependencies.**

## Design

### Convention
Subcommands = operations (distinct endpoints); flags = parameters. This removes the
mode-selector flags and replaces them with discoverable subcommands.

### Shared helper (cmd/purlcommon.go)
Add a small constructor so each PURL-list subcommand is one declaration:
```go
// newPurlServiceCmd builds a PURL-list subcommand that calls one SDK batch method.
func newPurlServiceCmd(use, short, long string, call decorateFunc) *cobra.Command {
    return &cobra.Command{
        Use:   use,
        Short: short,
        Long:  long,
        Args:  cobra.NoArgs,
        RunE:  func(cmd *cobra.Command, _ []string) error { return runPurlService(cmd, call) },
    }
}
```
Shared flags move from per-command `Flags()` to the **parent's `PersistentFlags()`** so
every subcommand inherits them. `addPurlServiceFlags` is updated to take a
`*pflag.FlagSet` (or a `*cobra.Command` and register on `PersistentFlags()`), and
`resolveComponents`/`runPurlService` keep reading via `cmd.Flags()` (persistent flags
are visible there).

### Per-service files
Each service file builds the parent and attaches subcommands in `init()`.

**vulnerabilities.go** — parent keeps a default op (`components`) for backward compat:
```go
var vulnerabilitiesCmd = &cobra.Command{
    Use: "vulnerabilities", Short: "...",
    RunE: func(cmd *cobra.Command, _ []string) error { // default = components
        return runPurlService(cmd, callVulnComponents)
    },
}
func init() {
    rootCmd.AddCommand(vulnerabilitiesCmd)
    addPurlServiceFlags(vulnerabilitiesCmd) // persistent
    vulnerabilitiesCmd.AddCommand(
        newPurlServiceCmd("components", "...", "...", callVulnComponents),
        newPurlServiceCmd("cpes",       "...", "...", callVulnCpes),
    )
}
```
where `callVulnComponents := func(c, ctx, comps){ return c.Vulnerabilities.Components(ctx, comps) }`.

**cryptography.go** — parent default op `algorithms`; 5 subcommands
(`algorithms`, `algorithms-range`, `versions-range`, `hints`, `hints-range`). Removes
`--algorithms/--libraries/--range` and `runCryptography`.

**geoprovenance.go** — parent default op `origin`; 2 subcommands (`origin`,
`countries`). Removes `--origin/--countries`.

**licenses.go** — parent default op `attribution`; 2 subcommands
(`attribution`, `evidence`).

**copyright.go (new)** — parent default op `evidence`; 2 subcommands (`evidence`,
`holders`).

**components.go (new)** — parent default op `search`; 3 subcommands:
- `search` (default) — bespoke `RunE`: reads `--search/--vendor/--component/--purl-type/--limit/--offset`,
  builds `scanoss.ComponentSearch`, calls `Components.Search`, writes `res.String()`.
  These flags live on the **parent** (so the bare `components` default works) and are
  inherited by the `search` subcommand. Uses the API/output flags but **not** the
  PURL-list flags.
- `versions` — bespoke `RunE`: reads `--purl` (single) + `--limit`, calls
  `Components.Versions(purl, limit)`.
- `status` — PURL-list (`newPurlServiceCmd`, calls `Components.Status`); registers the
  PURL-list flags locally on the subcommand.

For the bespoke `search`/`versions` runners, factor the client construction +
output write out of `runPurlService` into a tiny helper (e.g. `newClient(cmd)` +
`writeResult(cmd, res)`) so they don't duplicate the API-flag plumbing or progress
setup (search/versions are single GETs — no progress bar needed).

### Parent default + the mistyped-subcommand gotcha
Every parent has a default `RunE`. cobra passes an **unknown first arg as a
positional** to that `RunE`, so a mistyped subcommand would otherwise run the default
silently. Mitigation: set `Args: cobra.NoArgs` on every parent, so a stray positional
(`cryptography algorithmz`) errors with usage instead of silently defaulting.

## Files to modify / add
| File | Change |
|---|---|
| `cmd/purlcommon.go` | add `newPurlServiceCmd`; make shared flags registrable as persistent; factor `newClient`/`writeResult` helpers for the bespoke runners |
| `cmd/vulnerabilities.go` | parent + `components`/`cpes` subcommands (default op `components`) |
| `cmd/cryptography.go` | parent (default op `algorithms`) + 5 subcommands; drop mode flags + `runCryptography` |
| `cmd/geoprovenance.go` | parent (default op `origin`) + `origin`/`countries`; drop mode flags |
| `cmd/licenses.go` | parent (default op `attribution`) + `attribution`/`evidence` |
| `cmd/copyright.go` | **new**: parent (default op `evidence`) + `evidence`/`holders` |
| `cmd/components.go` | **new**: parent (default op `search`) + `search`/`versions`/`status` |
| `CHANGELOG.md` | note the CLI subcommand restructure (Changed/Added) |

## Testing strategy
- **Command wiring (unit, cobra):** for each parent, assert the expected subcommands
  are registered (`cmd.Commands()` names) and that shared flags are present on a
  subcommand (inherited persistent flags).
- **Routing (unit, httptest):** execute `rootCmd` with args
  (`SetArgs([]string{"cryptography","hints-range","--purl","pkg:a","--api-url",srv.URL})`)
  against a stub server that records `r.URL.Path`; assert the right v3 path is hit per
  subcommand. Covers components `search`/`versions` query params too.
- **Default ops & bad input:** a bare `scanoss <service>` runs its default op (per
  the Defaults table); an unknown subcommand (`cryptography algorithmz`) errors via
  `Args: cobra.NoArgs` instead of silently defaulting.
- `go build ./...`; `go vet ./cmd/`; `gofmt -l cmd/`; `go test ./...`.

## Alternatives considered
- **Keep the mode flags alongside subcommands** (additive, non-breaking) — rejected:
  two ways to do the same thing, and it keeps the anti-pattern around.
- **All parents help-only (no defaults)** — fully uniform (git/kubectl style), but a
  bare `scanoss vulnerabilities`/`licenses` would print help instead of running, a
  regression for today's flagless use. Rejected: every parent gets a default op
  instead (the bare command is an alias for the most common operation).
- **One flat command per operation** (`scanoss crypto-hints-range`) — rejected:
  loses the service grouping and discoverability that subcommands give.

## Engineering conventions
- **Conventional Commits**; **atomic commits** (review before each); **no
  AI/assistant references**; **short** imperative subjects. Every code change ships
  with tests.

## Risks & rollout
- **Breaking (pre-1.0, CLI):** the mode-selector flags are removed; scripts using
  `cryptography --libraries` / `geoprovenance --countries` must move to the
  subcommand form. Documented in the CHANGELOG.
- **Non-breaking:** flagless invocations keep working via the default op (per FR-005):
  `vulnerabilities`→`components`, `cryptography`→`algorithms`, `geoprovenance`→`origin`,
  `licenses`→`attribution` all match today's behaviour. Only the removed mode flags
  break.
