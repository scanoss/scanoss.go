# Spec — verbose logging (`--verbose`)

## Problem

The root command defines a persistent `-v, --verbose` flag (`cmd/root.go`), but
**nothing reads it** — it has no effect. There is no logging library; all
diagnostics are ad-hoc `fmt.Fprintln(os.Stderr, …)` calls (~50 sites across
`cmd/*`). When a scan, upload, or decoration call misbehaves there is no way to
get request/flow detail.

## Goal

Make `--verbose` turn on **debug-level structured logging** using the Go standard
library **`log/slog`** (no external dependency). Logs go to **stderr** so that
`stdout` stays clean for results/SBOM/JSON. Provide useful diagnostics for the
scan pipeline and API calls.

## Non-goals

- Replacing the progress bars (they remain the normal-run UX).
- External logging deps (logrus/zap/zerolog) — stdlib `slog` only.
- Log files, rotation, JSON-to-file, or remote log shipping.
- Changing `stdout` result output or exit codes.

## Behaviour

- **Default (no `-v`):** level **Warn** — only warnings and errors are logged to
  stderr; normal output is unchanged (results on stdout, progress bars on stderr).
- **`-v` / `--verbose`:** level **Debug** — operational (Info) and diagnostic
  (Debug) detail is emitted to stderr.
- Log format: `slog` text handler on stderr (human-readable `key=value`).
- The **SDK** (`pkg/scanoss`) must remain embeddable: it logs through a
  caller-supplied `*slog.Logger` (defaulting to `slog.Default()`), never forcing
  global state on its own.

## What gets logged (examples)

- **Debug:** files collected / skipped counts; fingerprint sizes; each HTTP
  request (method, URL, status, duration); scan id; chunk uploads; poll attempts
  and cadence; BOM removals; decoration chunking/worker counts.
- **Info:** high-level milestones (scan started/completed, N components decorated).
- **Warn:** recoverable issues (a file that failed to fingerprint, TLS verification
  disabled).
- **Error:** operations that fail.

## Acceptance

- `scanoss scan <path> -v` prints debug lines (collection, fingerprint, upload,
  poll, HTTP status) to stderr; without `-v` those are absent.
- `stdout` (results/SBOM) is byte-identical with and without `-v`.
- No new third-party dependency in `go.mod`.
