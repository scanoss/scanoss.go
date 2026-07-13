# Feature Specification: SCANOSS decoration pipeline

**Feature branch:** `feat/scanoss-decoration-sdk-pipeline`
**Status:** Draft
**Tracking issue:** internal

## Summary
Provide a way to run a configurable set of SCANOSS decoration services
(vulnerabilities, licenses, cryptography, geoprovenance) over the same list of
PURLs, concurrently, while reporting progress, and return a single object that
groups every service's output by service.

## User Scenarios & Testing

### Primary user story
As a developer using the `pkg/scanoss` SDK, I want to query several decoration
services for the same set of components in one call, watch overall progress, and
receive one combined result keyed by service — instead of calling each service
separately and stitching the outputs together myself.

### Acceptance scenarios
1. **Given** a list of PURLs and a pipeline configured with vulnerabilities,
   licenses, cryptography, and geoprovenance, **when** I run the pipeline,
   **then** I receive one object whose keys are the services and whose values are
   each service's full result.
2. **Given** a configured pipeline, **when** I add or remove a service before
   running, **then** the run includes exactly the services currently configured.
3. **Given** a running pipeline, **when** it executes, **then** the selected
   services are queried in parallel (not one-after-another).
4. **Given** a progress handler, **when** the pipeline runs, **then** I receive
   progress updates that identify which service advanced and how far
   (e.g. "vulnerabilities 50/100").
5. **Given** one service fails while others succeed, **when** the pipeline
   completes, **then** I receive the successful services' outputs plus a record of
   which service failed and why.
6. **Given** a pipeline running several services in parallel, **when** I call
   run, **then** the call returns only after **every** service has finished
   (success or failure) — no service is still running when run returns, and the
   wall-clock time is roughly that of the slowest service, not their sum.

### Edge cases
- No services configured → the run reports an error rather than silently doing nothing.
- Empty PURL list → the run reports an error (nothing to query).
- The same service added twice → it is queried only once.
- Caller cancels mid-run → in-flight work stops promptly and no new service calls start.
- Every service fails → the run returns an error (not an empty success).

## Requirements

### Functional
- **FR-001** The system MUST let a caller configure which decoration services a
  pipeline runs, starting from an initial set.
- **FR-002** The system MUST let a caller add and remove services from a pipeline
  before running it, ignoring duplicates.
- **FR-003** The system MUST run the configured services over the same input
  components **in parallel**.
- **FR-004** The system MUST report progress that identifies the service and its
  completion (units done / total), in a unit comparable across services.
- **FR-004a** The pipeline MUST expose progress as a **per-service snapshot**
  (a struct keyed by service, each entry holding that service's done/total/unit),
  suitable for rendering each service's progress individually on a UI. The
  pipeline MUST aggregate this internally and deliver it **serially** so a
  consumer never has to lock or aggregate the raw events itself.
- **FR-005** The system MUST return a single result that groups each service's
  output under that service's key.
- **FR-006** The system MUST report per-service failures without discarding the
  successful services' output; it MUST return an error only when every service
  failed.
- **FR-007** The system MUST honor caller cancellation: stop issuing new work and
  abort in-flight work.
- **FR-008** The pipeline input MUST be an array of components, where each
  component is a PURL with an optional version requirement
  (`[{ "purl": "…", "requirement": "…"? }]`).
- **FR-009** The SDK MUST provide an easy helper to convert an array of PURL
  strings into the component input type (with empty requirements), so callers who
  only have PURLs do not have to build the structs by hand.
- **FR-010** Running the pipeline MUST start all configured services
  concurrently and block until **every** service has completed (success or
  failure); it MUST NOT return while any service is still running. The pipeline
  is "finished" exactly when its last service finishes.

### Non-functional
- **NFR-001** The progress contract SHOULD be safe to consume when multiple
  services report concurrently.
- **NFR-002** The feature SHOULD reuse the existing chunking + concurrency engine
  rather than introducing a second request path.

## Out of scope
- A CLI command that drives the pipeline.
- Including the WFP scan as a pipeline stage.
- The dependencies service.

## Key entities
- **DecorationPipeline** — a configurable, ordered set of decoration services to run.
- **Component** — a PURL plus an optional version requirement.
- **DecorationPipeline result** — a mapping of service → that service's output, plus a
  record of any per-service failures.
- **Progress update** — service identity, units done, units total, unit label.
- **DecorationPipeline progress snapshot** — a struct keyed by service, each value a
  progress update; the full current state of every service's progress, for UI
  rendering.
