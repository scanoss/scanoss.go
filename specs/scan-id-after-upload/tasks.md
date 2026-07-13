# Tasks: Surface the scan id only after the full WFP is uploaded

**Spec:** `./spec.md` · **Plan:** `./plan.md` · **Issue:** [#40](https://github.com/scanoss/scanoss.go/issues/40)

## T1 — Move the notify after a successful upload  (`pkg/scanoss/scan.go`)
- [ ] In `upload()`, remove the `onScanID` call that fires before `uploadChunks`.
- [ ] After `uploadChunks` returns nil, call `s.c.onScanID(scanID)` (nil-guarded), then
  return the id. On upload error, return without notifying.
- [ ] Update the `upload` doc comment to reflect the new order (upload, then notify).
- *Done:* notify fires only on a fully-uploaded scan, exactly once, before polling.

## T2 — Fix the contract doc  (`pkg/scanoss/client.go`)
- [ ] Reword `WithScanIDNotify`: fires once **after** the full WFP is uploaded and
  before polling (id is resumable via `Scan.Wait`), not "before any chunk".
- [ ] Sanity-check the `ScanAPI` comment at `scan.go:22` still reads correctly.

## T3 — Tests  (`pkg/scanoss/scan_test.go`)
- [ ] Notify-after-upload: capture `mock.uploads` at notify time; assert it equals the
  total chunk count (multi-chunk via `WithChunkBytes(4)`).
- [ ] No-notify-on-failed-upload: chunk POST returns non-2xx → `Scan.WFP` errors and the
  callback never fired.
- [ ] Confirm `TestScanUploadAndPoll` / `TestScanChunkCarriesClientID` still pass.

## T4 — CLI interleaving check  (`cmd/scan.go`, only if needed)
- [ ] Manually verify the upload bar finishes before `Scan id:` prints (Open decision 1).
- [ ] If the line is injected mid-bar, finish the "Uploading WFP" bar at upload end;
  otherwise no change.

## T5 — Verify
- [ ] `go build ./... && go vet ./... && gofmt -l . && go test ./... -race` clean.
- [ ] Manual scan of a large project: upload bar → scan id + resume hint → server
  processing; interrupt during processing → `results <id>` resumes.

## Commit sequence
1. `docs: add scan-id-after-upload SDD plan` — `specs/scan-id-after-upload/*` (this).
2. T1+T2 — `fix(scan): surface scan id only after WFP fully uploaded`.
3. T3 (+T4 if changed) — `test(scan): cover scan-id-after-upload timing` (or fold into T1).
