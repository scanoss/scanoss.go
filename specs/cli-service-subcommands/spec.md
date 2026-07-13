# Feature Specification: per-service CLI subcommands

**Feature branch:** `feat/decoration-single-batch-requests`
**Status:** Draft

## Summary
The decoration CLI commands currently select *which* decoration to query with a
**mutually-exclusive boolean flag** (`cryptography --algorithms|--libraries [--range]`,
`geoprovenance --origin|--countries`). That overloads flags — which are meant for
*options of one operation* — to choose between *distinct operations* (each is a
separate API endpoint).

This feature reshapes each service command into a **parent command with one
subcommand per operation**, following the standard CLI convention (git, docker,
kubectl, `gh`): subcommands are the operations; flags are their parameters
(`--purl`, `--input`, `--chunk-size`, `--output`, …). It also adds CLI coverage for
operations that are reachable in the SDK but not from the CLI today (vulnerability
CPEs, crypto versions-in-range) and for the new **copyright** and **components**
services.

### Command surface (after)
```
scanoss vulnerabilities  components | cpes
scanoss cryptography     algorithms | algorithms-range | versions-range | hints | hints-range
scanoss geoprovenance    origin | countries
scanoss licenses         attribution | evidence
scanoss copyright        evidence | holders          (new command)
scanoss components       search | versions | status  (new command)
```

Each subcommand maps to exactly one SDK batch method:

| Command | Subcommand | SDK call |
|---|---|---|
| vulnerabilities | `components` | `Vulnerabilities.Components` |
| vulnerabilities | `cpes` | `Vulnerabilities.Cpes` |
| cryptography | `algorithms` | `Cryptography.Algorithms` |
| cryptography | `algorithms-range` | `Cryptography.AlgorithmsInRange` |
| cryptography | `versions-range` | `Cryptography.VersionsInRange` |
| cryptography | `hints` | `Cryptography.Hints` |
| cryptography | `hints-range` | `Cryptography.HintsInRange` |
| geoprovenance | `origin` | `Geoprovenance.Origins` |
| geoprovenance | `countries` | `Geoprovenance.Countries` |
| licenses | `attribution` | `Licenses.Attribution` |
| licenses | `evidence` | `Licenses.Evidence` |
| copyright | `evidence` | `Copyright.Evidence` |
| copyright | `holders` | `Copyright.Holders` |
| components | `search` | `Components.Search` |
| components | `versions` | `Components.Versions` |
| components | `status` | `Components.Status` |

### Defaults per service
"Default" = the operation that runs when the service command is invoked **without a
subcommand** (`scanoss <service> [flags]`). Every service has a default; the bare
command is an alias for that operation, and the other operations are explicit
subcommands.

| Service | Default (bare command) | Subcommands |
|---|---|---|
| `vulnerabilities` | **`components`** | `components`, `cpes` |
| `cryptography` | **`algorithms`** | `algorithms`, `algorithms-range`, `versions-range`, `hints`, `hints-range` |
| `geoprovenance` | **`origin`** | `origin`, `countries` |
| `licenses` | **`attribution`** | `attribution`, `evidence` |
| `copyright` | **`evidence`** | `evidence`, `holders` |
| `components` | **`search`** | `search`, `versions`, `status` |

Rationale: `vulnerabilities`→`components`, `cryptography`→`algorithms`,
`geoprovenance`→`origin` and `licenses`→`attribution` **match today's behaviour** (the
bare command, or the no-selector-flag case, already meant exactly that operation), so
nothing breaks for existing flagless invocations. `copyright`→`evidence` and
`components`→`search` are new services; their defaults are chosen as the most common
entry point. A bare `scanoss components` runs `search`, which errors if none of
`--search/--vendor/--component` is given (the SDK enforces "at least one").

### Final command reference (examples)
```sh
# Vulnerabilities — default op: components
scanoss vulnerabilities --purl 'pkg:npm/lodash'                 # = components
scanoss vulnerabilities components --input purls.txt
scanoss vulnerabilities cpes --purl 'pkg:npm/lodash'

# Cryptography — default op: algorithms
scanoss cryptography --purl 'pkg:github/angular/angular' --requirement '19.2.15'  # = algorithms
scanoss cryptography algorithms       --purl 'pkg:github/angular/angular' --requirement '19.2.15'
scanoss cryptography algorithms-range --purl 'pkg:github/angular/angular' --requirement '>1.3.6'
scanoss cryptography versions-range   --purl 'pkg:github/angular/angular' --requirement '>1.3.6'
scanoss cryptography hints            --purl 'pkg:github/angular/angular' --requirement '19.2.14'
scanoss cryptography hints-range      --purl 'pkg:github/angular/angular' --requirement '>1.3.6'

# Geoprovenance — default op: origin
scanoss geoprovenance --purl 'pkg:github/scanoss/engine' --requirement '5.4.7'   # = origin
scanoss geoprovenance origin    --purl 'pkg:github/scanoss/engine' --requirement '5.4.7'
scanoss geoprovenance countries --purl 'pkg:npm/lodash' --purl 'pkg:npm/express'

# Licenses — default op: attribution
scanoss licenses --purl 'pkg:npm/lodash'                        # = attribution
scanoss licenses attribution --input purls.txt
scanoss licenses evidence --purl 'pkg:npm/lodash'

# Copyright — default op: evidence
scanoss copyright --purl 'pkg:npm/lodash'                       # = evidence
scanoss copyright evidence --purl 'pkg:npm/lodash'
scanoss copyright holders  --purl 'pkg:npm/lodash'

# Components — default op: search; search/versions take their own flags
scanoss components --vendor angular --component angular --limit 20   # = search
scanoss components search   --vendor angular --component angular --limit 20
scanoss components versions --purl 'pkg:github/angular/angular' --limit 50
scanoss components status   --purl 'pkg:npm/lodash' --requirement '1.2.3'

# Shared flags work on every PURL-list subcommand
scanoss vulnerabilities components --input purls.txt --chunk-size 20 --workers 10 \
  --api-key "$KEY" --output out.json
```

## User Scenarios & Testing

### Primary user story
As a CLI user, when I want to query a specific decoration I run an explicit
subcommand (`scanoss cryptography hints-range --purl …`) and discover the
available operations via `--help`, instead of memorising which flags select which
endpoint.

### Acceptance scenarios
1. **Given** a service with subcommands, **when** I run `scanoss <service> --help`,
   **then** I see the list of subcommands (operations).
2. **Given** a subcommand, **when** I run `scanoss <service> <op> --purl X`,
   **then** the CLI calls that one SDK method and writes the merged result, reusing
   the existing chunking/progress/output behaviour.
3. **Given** `components search`, **when** I pass `--search`/`--vendor`/`--component`
   (plus optional `--purl-type`/`--limit`/`--offset`), **then** the CLI issues the
   search GET (not the PURL-list batch path).
4. **Given** `components versions`, **when** I pass `--purl` (and optional `--limit`),
   **then** the CLI lists that component's versions.
5. **Given** the shared flags (`--purl`, `--input`, `--chunk-size`, `--workers`,
   `--api-url`, `--api-key`, `--ignore-cert-errors`, `--output`), **when** used on any
   PURL-list subcommand, **then** they behave exactly as today.

### Edge cases
- Unknown subcommand (`cryptography algorithmz`) → cobra prints "unknown command"
  and usage (no silent fallback). See FR-005 on parent defaults.
- `components search` with none of `--search/--vendor/--component` → error (the SDK
  already enforces "at least one").
- A service parent invoked with no subcommand → see FR-005.

## Requirements

### Functional
- **FR-001** Each decoration service MUST be a parent `cobra.Command` whose
  subcommands are its operations (one subcommand per SDK batch method in scope).
- **FR-002** Flags MUST be parameters only. The mode-selector flags
  (`--algorithms`, `--libraries`, `--range`, `--origin`, `--countries`) are
  **removed**; the operation is chosen by subcommand.
- **FR-003** The PURL-list subcommands MUST reuse the existing shared flags and the
  `runPurlService` path (resolve components → client + progress → call → write),
  with no behavioural change to chunking, progress, partial-failure reporting, or
  output.
- **FR-004** The `components` command MUST support operations with non-PURL-list
  inputs: `search` (`--search/--vendor/--component/--purl-type/--limit/--offset`) and
  `versions` (`--purl/--limit`); `status` uses the shared PURL-list flags.
- **FR-005** Every service parent MUST run a **default operation** when invoked
  without a subcommand, per the Defaults table (`vulnerabilities`→`components`,
  `cryptography`→`algorithms`, `geoprovenance`→`origin`, `licenses`→`attribution`,
  `copyright`→`evidence`, `components`→`search`). The parent MUST reject a stray
  positional arg (a mistyped subcommand) rather than silently running the default —
  use `Args: cobra.NoArgs` on the parent so an unknown subcommand errors.
- **FR-006** New top-level commands `copyright` and `components` MUST be registered
  on the root command.

### Non-functional
- **NFR-001** Subcommand wiring MUST minimise boilerplate (a shared helper builds a
  PURL-list subcommand from a name, short/long help, and the SDK closure).
- **NFR-002** Help text MUST be self-documenting at every level (`<service> --help`
  lists operations; `<service> <op> --help` lists that op's flags).

## Out of scope
- Single-component (`GET`) lookups from the CLI (the CLI stays batch; the SDK keeps
  its single methods). Could be a later `--one`/dedicated flow.
- Typed CLI output (still the merged JSON string via `Result.String()`).
- Cryptography ruleset download, dependencies, and the `*-contents` endpoints.

## Key entities
- **Service parent command** — a `cobra.Command` grouping a service's operations;
  registers the shared PURL-list flags as **persistent** flags so subcommands inherit
  them (except `components`, whose ops take different inputs).
- **Operation subcommand** — a `cobra.Command` with a `RunE` that calls one SDK
  method via the existing `runPurlService` (PURL-list ops) or a bespoke runner
  (`components search`/`versions`).
- **decorateFunc / runPurlService** — unchanged plumbing reused by every PURL-list
  subcommand.
