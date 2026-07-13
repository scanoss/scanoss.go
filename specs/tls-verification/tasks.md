# Tasks: TLS certificate verification toggle

**Spec:** [`spec.md`](./spec.md) · **Plan:** [`plan.md`](./plan.md)

`[P]` = parallelizable (different files). Paths relative to repo root.

## Conventions
- Conventional Commits; atomic; short subjects; no AI/co-author trailers.
- Every code task ships with unit tests.

## Phase 1 — SDK / client plumbing
- [ ] **T001 [P]** `pkg/scanoss/client.go`: add `WithInsecureTLS(insecure bool)
      Option` building an `http.Client` with `InsecureSkipVerify` transport;
      import `crypto/tls`. Unit test: option true → client transport has
      `InsecureSkipVerify == true`; false → default client unchanged.
- [ ] **T002 [P]** `pkg/api/client.go`: add `(*Client) SetInsecureTLS(insecure
      bool)` with the same transport; import `crypto/tls`. Unit test mirrors T001.

## Phase 2 — CLI wiring
- [ ] **T003** `cmd/purlcommon.go`: register `--ignore-cert-errors` in
      `addPurlServiceFlags`; in `runPurlService` read it, append
      `scanoss.WithInsecureTLS(v)` to `scanoss.New(...)`, print stderr warning
      when true. (depends on T001)
- [ ] **T004** `cmd/scan.go`: register `--ignore-cert-errors`; read it; call
      `apiClient.SetInsecureTLS(v)` after construction; print stderr warning when
      true. (depends on T002)

## Phase 3 — Docs & verification
- [ ] **T005 [P]** `README.md`: document `--ignore-cert-errors` for scan and the
      decoration commands; mark insecure.
- [ ] **T006 [P]** `CHANGELOG.md`: `[Unreleased] → Added` entry.
- [ ] **T007** End-to-end check against a self-signed endpoint: command fails
      without the flag (x509), succeeds with it and prints the warning; repeat
      for `scan`. `go build ./...` + `go vet ./...` clean.
