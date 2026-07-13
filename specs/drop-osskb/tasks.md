# Tasks: Drop osskb.org — default to api.scanoss.com + clear auth errors

**Spec:** `./spec.md` · **Plan:** `./plan.md` · **Issue:** [#37](https://github.com/scanoss/scanoss.go/issues/37)

## T1 — Collapse default endpoint to api.scanoss.com  (CLI config/cleanup)
- [ ] `internal/config/config.go`: `DefaultAPIURL = "https://api.scanoss.com"`; remove
  `PremiumAPIURL`; fix the doc comment (drop "free public OSS KB").
- [ ] `cmd/scan.go`: delete the premium-substitution block (`apiKey != "" &&
  !Changed("api-url")` → `PremiumAPIURL` + "using premium endpoint" print).
- [ ] Confirm `results.go` / `attributions.go` flag defaults (`config.DefaultAPIURL`)
  and `buildResultsCommand`'s `!= config.DefaultAPIURL` comparison still read correctly.
- *Done:* build/vet/test green; no `osskb` in any `.go` file; default endpoint uniform.
- → commit `refactor(cli): default to api.scanoss.com, drop osskb/premium split`.

## T2 — No-key pre-flight guard  (`cmd/auth.go`)
- [ ] `scanossNoKeyBanner` const (FR-007 ASCII banner → https://www.scanoss.com).
- [ ] `normalizeURL(u)` (trim space + trailing `/`, matching SDK `WithAPIURL`).
- [ ] `errNoAPIKey` sentinel; `requireKeyForDefaultEndpoint(apiURL, apiKey) error`:
  key set → nil; URL ≠ default → nil (on-prem keyless); else print banner + return
  sentinel.
- [ ] Wire into `RunE`s after flags are read: `scan` (folder + `--wfp`/`uploadAndWrite`),
  `results`, `attributions`, `runPurlServiceTyped`, components search/versions.
- [ ] Silence cobra usage/error dump for the sentinel (banner is the whole message).
- *Done:* default+no-key fails fast with banner, no network; custom URL keyless proceeds.
- → commit `feat(cli): guard default endpoint with no-key banner (on-prem exempt)`.

## T3 — Typed 401 + clear Unauthorized render  (SDK + CLI)
- [ ] `pkg/scanoss/transport.go`: `type StatusError struct { StatusCode int; Body string }`
  + `Error()` (text identical to today); return `&StatusError{...}` from `do` on non-2xx.
- [ ] `cmd/auth.go` (or `cmd/errors.go`): `renderAPIError(err)` — `errors.As` →
  `StatusError`; on 401 print clear "Unauthorized: missing/invalid API key …" to stderr;
  return err (keep non-zero exit).
- [ ] Route the scan, results, attributions, and purl-service error returns through
  `renderAPIError`.
- *Done:* a 401 from any keyed/custom request prints the clear message; non-401 unchanged.
- → commit `feat: surface 401 Unauthorized clearly via typed transport error`.

## T4 — Docs scrub
- [ ] `README.md`: replace osskb default-URL mentions (lines ~99, 103, 322, 669) with
  `https://api.scanoss.com`; drop the premium-endpoint note (no longer applies).
- [ ] `libscanoss/` docs (`README.md`, `docs/*.md`): update `api_url` examples off osskb.
- [ ] Leave historical `CHANGELOG.md` entry (line ~199) as-is pending Open decision 1;
  add a new CHANGELOG entry for this change.
- [ ] `grep -rni osskb .` → only approved historical lines remain.
- → commit `docs: scrub osskb.org references`.

## T5 — Tests & verify
- [ ] `cmd/auth_test.go`: table test for `requireKeyForDefaultEndpoint` (default+no-key,
  default+key, custom+no-key, explicit-default-with-trailing-slash+no-key).
- [ ] `pkg/scanoss/`: assert a 401 response is a `*StatusError` with `StatusCode==401`
  (`errors.As`).
- [ ] `cmd/subcommands_test.go`: default-endpoint + no-key command exits non-zero, prints
  banner, makes no request.
- [ ] `go build ./... && go vet ./... && gofmt -l . && go test ./... -race` clean.

## Commit sequence
1. `docs: add drop-osskb SDD plan` — `specs/drop-osskb/*` (this).
2. T1 — `refactor(cli): default to api.scanoss.com, drop osskb/premium split`.
3. T2 — `feat(cli): guard default endpoint with no-key banner (on-prem exempt)`.
4. T3 — `feat: surface 401 Unauthorized clearly via typed transport error`.
5. T4 — `docs: scrub osskb.org references`.
(Tests land with their slice or as a final T5 commit.)
