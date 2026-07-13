<!-- Migrated from openspec/changes/rename-scan-routes/design.md -->

# Design: Rename scan routes to `<resource>/scan`

## Context

The v3 scan endpoints are being inverted from `scan/<action>` to
`<resource>/scan`. The full backend rename is:

- `/v3/scan/batch` → `/v3/wfp/scan`
- `/v3/scan/raw` → `/v3/raw/scan`
- `/v3/scan/snippets` → `/v3/snippets/scan`
- `/v3/scan/components` → `/v3/components/scan`

In this client, a grep for those paths finds references only to **`scan/batch`**:

- `pkg/scanoss/scan_transport.go:14` — `ServiceScan = Service{name: "scan", endpoint: "/v3/scan/batch"}` (the v3 SDK path; upload + poll both derive from this).
- `pkg/api/strategies.go:41` — `baseURL + "/scan/batch"` (legacy, non-`/v3`).
- `pkg/api/client.go:260` — `c.apiURL + "/scan/batch/" + sessionID` (legacy poll).

No references exist for `scan/raw`, `scan/snippets`, or `scan/components`, so the
client change is limited to the batch endpoint.

## Goals / Non-Goals

Goals:
- Point the SDK's batch scan at `/v3/wfp/scan` (upload and poll).
- Keep request semantics identical (method, `X-Scan-Id`/`Content-Range` headers,
  body, polling cadence).

Non-Goals:
- Renaming the backend routes (separate `scanoss.api` change).
- Adding client support for the three unreferenced routes.

## Decisions

### v3 SDK path (`pkg/scanoss/scan_transport.go`)
Change the single endpoint constant:

```go
var ServiceScan = Service{name: "scan", endpoint: "/v3/wfp/scan"}
```

The SDK builds both the POST upload URL (`scan_transport.go uploadChunk`) and the
`GET ...?id=` poll URL (`scan.go Status`) from `ServiceScan.endpoint`, so this one
change moves both. The `name: "scan"` tag (used for progress/logging) can stay or
become `"wfp.scan"` — cosmetic; keep `"scan"` to avoid churning progress labels.

### Legacy batch client (`pkg/api/`)
`pkg/api/strategies.go` and `client.go` use a non-`/v3` `/scan/batch`. Two options:
1. **Rename to `/wfp/scan`** to match — correct if this client still targets the
   renamed backend.
2. **Leave as-is** if the legacy `/scan/batch` is served by a different (v2)
   backend not affected by this rename.

Decide by confirming which backend `pkg/api` targets. Default: rename to
`/wfp/scan` for consistency, and verify against the backend before merging.

### Polling URL shape
Today the v3 poll is `GET /v3/scan/batch?id=<id>` (`scan.go Status` → `c2url`).
After the rename it becomes `GET /v3/wfp/scan?id=<id>`. The query-param form is
unchanged; only the path segment moves. (The legacy `pkg/api` client uses a path
suffix `/scan/batch/{id}` instead — that becomes `/wfp/scan/{id}` if renamed.)

## Risks / Trade-offs

- **Lockstep breaking change**: the client and backend must agree on the path. A
  client on the new path against an old backend (or vice-versa) gets 404. Roll out
  together, or have the backend serve both paths during a transition window.
- **Low blast radius in code**: one constant for v3, two lines for the legacy
  client. The risk is coordination, not implementation.

## Verification

- Grep confirms no remaining `/scan/batch` references after the change (except
  intentionally retained legacy ones, if any).
- `go build ./...`, `go vet ./...`, `go test ./...` green.
- Manual: run a scan against a backend serving `/v3/wfp/scan` and confirm upload +
  poll + result.
