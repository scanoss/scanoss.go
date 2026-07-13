# Plan — verbose logging (`--verbose`)

## Library

`log/slog` (standard library, Go 1.21+). No `go.mod` change.

## Design

### `internal/logging`
A small package that owns logger setup:

```go
// Configure builds a text slog.Logger on stderr at the given level and installs
// it as the default. verbose => Debug, otherwise Warn.
func Configure(verbose bool) *slog.Logger {
    level := slog.LevelWarn
    if verbose {
        level = slog.LevelDebug
    }
    h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
    l := slog.New(h)
    slog.SetDefault(l)
    return l
}
```

### CLI wiring
In `cmd/root.go`, add a `PersistentPreRunE` that reads `--verbose` and calls
`logging.Configure(verbose)` once, before any subcommand runs. Every command
inherits it.

### SDK (`pkg/scanoss`) — stays embeddable
- Add `WithLogger(*slog.Logger) ClientOption`; the client stores a `*slog.Logger`
  defaulting to `slog.Default()`.
- SDK code logs via `c.log.Debug(...)` etc. — no direct global use, so embedders
  control output. The CLI's `slog.SetDefault` flows through the default.

### Log points
- `pkg/scanner`: files collected / skipped; per-file fingerprint failures (Warn);
  total WFP size (Debug).
- `pkg/api` / `pkg/scanoss` transport: each request — method, URL, status,
  duration (Debug); `Retry-After` waits (Debug/Warn).
- `pkg/scanoss` scan flow: scan id (Debug/Info), chunk count + per-chunk upload
  (Debug), poll attempts + interval (Debug), completion (Info).
- `pkg/scanoss` decoration pipeline: services, chunk size, worker count (Debug).
- BOM post-processing: components removed (Debug).

### Migrate ad-hoc stderr
Convert **diagnostic** `fmt.Fprintln(os.Stderr, …)` to the matching `slog` level
(e.g. "Warning: ignoring TLS certificate errors" → `slog.Warn`). Keep genuinely
user-facing lines (resume-command hint, "Filtered N files" summary) as prints, or
move to `slog.Info` — decided per-site during implementation, favouring the least
behavioural change to normal runs.

## Testing

- `internal/logging`: table test that `Configure(false)` → Warn enabled/Debug
  disabled, `Configure(true)` → Debug enabled (capture via a buffer handler).
- A CLI-level check (or manual, per the repo's `verify` habit): `scan … -v` emits
  debug lines; without `-v` it does not; stdout identical both ways.

## Risks / notes

- Keep default at **Warn** so normal runs don't get noisier.
- Ensure logs never go to **stdout** (would corrupt piped JSON/SBOM).
- `slog.SetDefault` is process-global; that's fine for the CLI. The SDK avoids
  depending on it by taking `WithLogger`.
