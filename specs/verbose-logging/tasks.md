# Tasks — verbose logging (`--verbose`)

Atomic, one commit each; tree builds and `make check` stays green after every step.

- [x] **T1 — `internal/logging` package.** Add `Configure(verbose bool) *slog.Logger`
  (text handler on stderr; Debug when verbose, else Warn; `slog.SetDefault`).
  Unit test the level mapping with a buffer handler.

- [x] **T2 — Wire `--verbose` in the CLI.** Add `PersistentPreRunE` to the root
  command that reads the `verbose` flag and calls `logging.Configure`. Verify
  `-v` produces debug output and the default does not.

- [x] **T3 — SDK `WithLogger`.** Add `WithLogger(*slog.Logger)` to `pkg/scanoss`;
  store a `*slog.Logger` on the client defaulting to `slog.Default()`. No behaviour
  change when unused.

- [x] **T4 — Scan-flow + transport debug logs.** Add Debug/Info/Warn logs in
  `pkg/scanner` (collect/fingerprint) and the `pkg/scanoss` scan flow + HTTP
  transport (request method/URL/status/duration, scan id, chunks, polling).

- [x] **T5 — Decoration + filter + BOM logs.** Add Debug logs to the decoration
  pipeline (services, chunk size, workers), file filtering (skip counts), and
  BOM post-processing (components removed).

- [x] **T6 — Migrate ad-hoc stderr diagnostics.** Convert diagnostic
  `fmt.Fprintln(os.Stderr, …)` to the appropriate `slog` level; keep genuine
  user-facing lines. No change to normal-run stdout.

- [x] **T7 — Docs.** Add a "Verbose / logging" note to `CLIENT_HELP.md` (and the
  global-options list) and a CHANGELOG entry under `## [Unreleased]`.
