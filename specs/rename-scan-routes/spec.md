<!-- Migrated from openspec/changes/rename-scan-routes (proposal + capability spec). Work already shipped. -->

# Proposal: Rename scan routes to `<resource>/scan`

## Why

The SCANOSS v3 API is moving its scan endpoints from an `scan/<action>` layout to
a `<resource>/scan` layout (resource-first), to be consistent with the rest of the
API surface where the resource leads the path:

| Old | New |
|---|---|
| `/v3/scan/batch` | `/v3/wfp/scan` |
| `/v3/scan/raw` | `/v3/raw/scan` |
| `/v3/scan/snippets` | `/v3/snippets/scan` |
| `/v3/scan/components` | `/v3/components/scan` |

This client (`scanoss`) must follow the backend so the SDK keeps reaching the
right endpoints. The client only **consumes** these routes; the definitions live
in the backend (`scanoss.api`), which is renamed separately.

## What Changes

Of the four routes, **only `scan/batch` is referenced by this client** — the SDK's
batch scan path. The other three (`raw`, `snippets`, `components`) have no client
references today, so this change is scoped to the batch endpoint:

- `pkg/scanoss/scan_transport.go` — `ServiceScan.endpoint` `/v3/scan/batch` → `/v3/wfp/scan`.
- `pkg/api/strategies.go` and `pkg/api/client.go` — the legacy (non-`/v3`) batch
  client's `/scan/batch` and `/scan/batch/{id}` paths, kept consistent with the
  rename (decide per backend: drop, or move to `/wfp/scan`).

The polling path (`GET .../{id}`) moves with the upload path.

## Impact

**Modified Packages:**
- `pkg/scanoss/scan_transport.go` — v3 batch endpoint constant.
- `pkg/api/strategies.go`, `pkg/api/client.go` — legacy batch endpoint paths (if still in use).

**Behavior:**
- The SDK's request method, headers (`X-Scan-Id`, `Content-Range`), body, and
  polling cadence are unchanged — only the URL path changes.
- No CLI flag changes, no result-shape changes.

**Compatibility:**
- This is a **breaking** coordination with the backend: the client must point at
  the new path, and the backend must serve it. Old and new clients are not
  interchangeable across the rename. Roll out in lockstep with the backend, or
  gate behind a version negotiation if the backend serves both temporarily.

**Out of scope:**
- `raw`, `snippets`, `components` scan routes — not referenced by this client.
  They are renamed in the backend; this client gains references only if/when it
  starts calling them.
- The backend route definitions themselves (separate change in `scanoss.api`).


---

## Capability spec: scan-workflow

# Scan Workflow

## MODIFIED Requirements

### Requirement: Batch Mode with Session Tracking
The system MUST support batch mode for large scans with server-side processing, using the resource-first scan route layout (`/v3/wfp/scan`).

#### Scenario: Scan in batch mode
- **GIVEN** scan command with `--batch` flag
- **WHEN** processing files
- **THEN** system generates unique session ID
- **AND** sends fingerprints to the `/v3/wfp/scan` endpoint
- **AND** includes Session-Id header in all requests
- **AND** polls for results after all chunks sent

#### Scenario: Poll for batch results
- **GIVEN** batch scan with all chunks sent
- **WHEN** polling for results
- **THEN** system polls `/v3/wfp/scan?id={session-id}` every 5 seconds
- **AND** displays server processing progress
- **AND** retrieves final results when complete

#### Scenario: Allow early exit from polling
- **GIVEN** batch scan polling in progress
- **WHEN** user presses CTRL+C
- **THEN** system exits gracefully
- **AND** displays command to retrieve results later
- **AND** shows session ID for future use
