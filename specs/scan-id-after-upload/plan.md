# Implementation Plan: Surface the scan id only after the full WFP is uploaded

**Spec:** `./spec.md`
**Status:** Draft

## Approach
A single-point timing change in the SDK, plus a doc-comment fix and tests. The scan id
is already generated and threaded through chunks correctly; the only defect is that the
caller is *notified* too early. Move the one `onScanID` call from before `uploadChunks`
to after it succeeds. No CLI logic change is required (the hook body stays the same) —
the message simply fires at the right moment.

## Mechanics

### SDK — move the notify (`pkg/scanoss/scan.go`, `upload()`)
Current (lines ~167-181):
```go
scanID := id.String()
if s.c.onScanID != nil {
    s.c.onScanID(scanID)        // <-- fires BEFORE upload
}

ranges := chunkRanges(len(wfp), chunkBytes)
prog := &chunkProgress{c: s.c, total: len(ranges)}
if err := s.uploadChunks(ctx, scanID, wfp, ranges, prog); err != nil {
    return "", err
}
return scanID, nil
```
Target:
```go
scanID := id.String()

ranges := chunkRanges(len(wfp), chunkBytes)
prog := &chunkProgress{c: s.c, total: len(ranges)}
if err := s.uploadChunks(ctx, scanID, wfp, ranges, prog); err != nil {
    return "", err            // upload failed → nothing resumable, no notify
}

// The full WFP is now on the server: the id is resumable. Surface it before
// polling so an interrupted scan can be recovered with `results <id>`.
if s.c.onScanID != nil {
    s.c.onScanID(scanID)
}
return scanID, nil
```
Update the `upload` doc comment ("generates the scan id … fires the notify hook, then
uploads") to reflect the new order.

### SDK — fix the contract doc (`pkg/scanoss/client.go`, `WithScanIDNotify`)
Replace "invoked once, as soon as the client generates the scan id (before any chunk is
uploaded)" with wording like: "invoked once, after the full WFP has been uploaded and
before polling begins — at which point the id is resumable via `Scan.Wait`." Touch the
related comment at `scan.go:22` if needed (it only says "surfaced via WithScanIDNotify
for optional recovery" — still accurate).

### CLI — no logic change (`cmd/scan.go`)
The `WithScanIDNotify` hook body is unchanged. Open decision 1: confirm the
upload-progress bar is finished (newline) before the id prints. The hook already writes
a leading `\n`; the "Uploading WFP" bar is `Finish()`ed lazily on the first `phase`
update inside `scanProgress.fn`, which happens *after* the notify. If the interleaving
looks off in manual testing, finish the upload bar at the end of upload (e.g. emit a
terminal `chunks` update or finish on notify) — keep it minimal.

## File-by-file
**Modified**
- `pkg/scanoss/scan.go` — move the `onScanID` call after `uploadChunks`; update
  `upload` doc comment.
- `pkg/scanoss/client.go` — correct the `WithScanIDNotify` doc comment.

**Tests**
- `pkg/scanoss/scan_test.go` — (a) assert the notify fires only after all chunks are
  uploaded; (b) assert no notify on a failed upload.

## Tests
- **Notify-after-upload:** in the scan mock, capture `atomic.LoadInt32(&mock.uploads)`
  at the moment `onScanID` fires and assert it equals the total chunk count (all chunks
  already received). Reuse the multi-chunk setup from `TestScanUploadAndPoll`
  (`WithChunkBytes(4)` → 4 chunks).
- **No-notify-on-failed-upload:** a mock whose chunk POST returns a non-2xx → `Scan.WFP`
  returns an error and the `onScanID` callback was never invoked (a bool stays false).
- Existing `TestScanUploadAndPoll` / `TestScanChunkCarriesClientID` still pass: they
  only assert `gotID` is non-empty and equals the carried id after completion — timing
  change is compatible.

## Risks / notes
- **Public contract change.** `WithScanIDNotify` timing moves from "before upload" to
  "after upload". No valid caller can rely on the old timing (a pre-upload id is not
  resumable), so this is a correctness fix, not a regression. Doc updated accordingly.
- **No id on interrupted upload.** Intentional: there is nothing to resume. The id is
  still surfaced for the genuine recovery window (server-processing/poll phase).
- **Single call site.** All three entry points (`Folder`, `Files`, `WFP`) funnel through
  `scan` → `upload`, so the one move covers every path.

## Verification
- `go build ./... && go vet ./... && gofmt -l . && go test ./... -race` clean.
- Manual: `scanoss scan ./large-proj` → the "Uploading WFP" bar completes, **then**
  `Scan id: …` + resume hint print, **then** "Server processing" begins. Interrupt
  during processing and confirm `scanoss results <id>` resumes successfully.
