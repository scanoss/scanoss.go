# Feature Specification: Surface the scan id only after the full WFP is uploaded

**Feature branch:** `fix/scan-id-after-upload`
**Issue:** [#40](https://github.com/scanoss/scanoss.go/issues/40)
**Status:** Draft

## Summary
Today the CLI prints the scan id and a "resume with `scanoss results <id>`" hint
**before** the WFP has been uploaded. The id is generated client-side and the notify
hook fires immediately, so the recovery command is shown while chunks are still in
flight. A scan id is only safely resumable once its **entire** WFP is on the server —
resuming a half-uploaded scan yields incomplete or wrong results.

Move the scan-id notification so it fires **after** all WFP chunks have uploaded
successfully and **before** polling begins. The id (and the recovery command) then
appears only when it genuinely points at a fully-uploaded, resumable scan.

### Current state (why this is needed)
- `pkg/scanoss/scan.go:162-182` — `upload()` generates the UUIDv7, fires
  `s.c.onScanID(scanID)` (line 172-174) **before** `uploadChunks`, then uploads.
- `pkg/scanoss/client.go:163-166` — `WithScanIDNotify` is documented as firing "as soon
  as the client generates the scan id (**before any chunk is uploaded**)".
- `cmd/scan.go:330-333` — the CLI hook prints `Scan id: <id>` + `If interrupted, resume
  with: scanoss results <id>` the moment the hook fires.
- Net effect: the resume affordance is shown during upload, when the id is **not** yet
  resumable. Interrupting and resuming mid-upload produces a partial/incorrect scan.

## User Scenarios & Testing

### Primary user story
As a user running `scanoss scan ./proj`, I only see a scan id and a resume command
once my fingerprints are fully uploaded — so if I copy that command it always points at
a scan the server can actually resume.

### Acceptance scenarios
1. **Given** a multi-chunk WFP, **when** I scan, **then** the scan id is printed only
   after every chunk has been uploaded (not before/during upload), and before the
   server-processing phase.
2. **Given** the upload fails partway (e.g. a chunk errors), **then** **no** scan id /
   resume command is printed — there is nothing resumable.
3. **Given** the upload succeeds but the scan later fails or is interrupted during
   server processing, **then** the printed id is valid and `scanoss results <id>`
   resumes it (the full WFP is on the server).
4. **Given** a registered `WithScanIDNotify` callback, **then** it fires exactly once,
   after a successful upload and before `Wait` polls.

### Edge cases
- Single-chunk WFP → notify still fires after that one chunk uploads, not before.
- Context cancelled during upload → upload returns the cancellation error; notify does
  not fire (scenario 2 generalizes).
- `Wait` fails after a good upload → id was already surfaced (correct: it is resumable).

## Requirements

### Functional
- **FR-001** The SDK fires `onScanID` only **after** `uploadChunks` returns success,
  immediately before `Wait` begins polling. If upload fails, `onScanID` must not fire.
- **FR-002** The notify still fires **exactly once** per scan when an upload succeeds.
- **FR-003** Update the `WithScanIDNotify` contract doc (`client.go`) to state the hook
  fires after the full WFP is uploaded (and before polling), not before any chunk.
- **FR-004** No change to the scan id value, the `X-Scan-Id` header on chunks, or the
  resume mechanics — only the **timing** of the notification.
- **FR-005** The CLI message remains accurate: the id + resume hint print after upload,
  ahead of the "Server processing" bar, without garbling the progress bars.

### Non-functional
- **NFR-001** No change to upload concurrency, polling, or result handling.
- **NFR-002** Build, vet, gofmt-clean, tests pass (incl. `-race`).

## Open decisions
1. **CLI progress interleaving.** The notify now lands between the finished "Uploading
   WFP" bar and the "Server processing" bar. **Recommend** ensuring the upload bar is
   explicitly finished (newline emitted) before the id is printed, so the line isn't
   injected mid-bar. _Confirm whether to adjust `scanProgress` or rely on the existing
   leading `\n` in the hook._
2. **Message wording.** With correct timing, "If interrupted, resume with…" is now
   accurate for the processing phase. **Recommend** keeping the wording (optionally
   "Uploaded. Scan id: …"). _Confirm._

## Out of scope
- Server-side partial-upload detection or cleanup of abandoned ids.
- Any change to fingerprinting, chunking, or the `results`/`Wait` flow itself.
- Resuming an interrupted **upload** (only a fully-uploaded scan is resumable).
