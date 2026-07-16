# Feature Specification: rename the `convert` command to `sbom`

**Feature branch:** `feat/rename-convert-to-sbom`
**Issue:** [#11](https://github.com/scanoss/scanoss.go/issues/11)
**Status:** Draft

## Summary
Rename the offline format-conversion command from `convert` to **`sbom`**. Behavior is
**identical** — it still reads a scanoss raw result, CycloneDX, or SPDX (detected from content)
and writes the target format via the neutral `sbom.Inventory` — only the command name (and its
docs/tests) change.

```
# before
scanoss convert bom.spdx.json --format cyclonedx -o bom.cdx.json
# after
scanoss sbom     bom.spdx.json --format cyclonedx -o bom.cdx.json
```

## Rationale
- `sbom` names the *outcome* the user wants — "produce an SBOM (in this format) from what I
  already have" — which reads more clearly than the generic verb `convert`.
- It sits naturally beside the format vocabulary the CLI already uses (`--format
  cyclonedx|spdx`) and the `pkg/sbom` model that backs it.
- **Non-breaking:** `convert` is **not** in the released `v0.1.0` tag (it was added afterward), so
  no shipped CLI exposes `convert` to users — the rename touches only unreleased surface.

## Command surface (unchanged except the name)
```
scanoss sbom <input> --format <cyclonedx|spdx> [--output <file>]
```
- `<input>` — a raw scanoss v3 result, a CycloneDX JSON, or an SPDX JSON (content-detected).
- `-f, --format` — target: `cyclonedx` or `spdx` (required; `raw` is not a conversion target).
- `-o, --output` — output file (default: stdout).
- Offline: no API, no scanning, no auth flags.

## User scenarios & acceptance
1. `scanoss sbom x.cdx.json --format spdx` produces a valid SPDX 2.3 document (was `convert`).
2. `scanoss sbom x.spdx.json --format cyclonedx` produces a valid CycloneDX 1.7 document.
3. `scanoss sbom result.json --format cyclonedx|spdx` converts a raw scan result.
4. Invalid/unrecognized input and invalid `--format` still error the same way.
5. `scanoss convert …` no longer resolves as a command (no alias — hard rename).
6. `--help` and the docs show `sbom`, not `convert`.

## Requirements
### Functional
- **FR-001 (command rename)** Rename the cobra command to `Use: "sbom <input>"`, and the
  identifiers `convertCmd → sbomCmd`, `runConvert → runSbom`. Short/Long help updated to say
  `sbom`. Semantics unchanged.
- **FR-002 (file rename)** `cmd/convert.go → cmd/sbom.go`, `cmd/convert_test.go →
  cmd/sbom_test.go` (via `git mv`, preserving history). Internal test helpers renamed to match
  (`runConvertTest → runSbomTest`, `TestConvert_* → TestSbom_*`).
- **FR-003 (docs)** Update the command name in `README.md` (Commands table), `CLIENT_HELP.md`
  (table-of-contents anchor + section heading + examples), and `CHANGELOG.md` (the `convert`
  entry becomes `sbom`).
- **FR-004 (no behavior change)** Format identification, conversion through `sbom.Inventory`,
  lossy-layer warnings, and target validation are untouched. Only names change.

### Non-functional
- **NFR-001** `make check` clean; all renamed tests pass unchanged in substance.
- **NFR-002** No new dependencies; no change to `pkg/sbom` or `scansource`.

## Open decisions
1. **Keep `convert` as a deprecated alias?** _Resolved: no._ Hard rename with **no alias** —
   `convert` is unreleased so nothing depends on it, and keeping the surface clean is preferred.
   `scanoss convert` will no longer resolve after the rename.
2. **CHANGELOG placement.** The current `convert` entry sits under the `## [0.1.0]` heading even
   though `convert` is not in the `v0.1.0` tag. **Recommendation:** rename that entry in place to
   `sbom` (it documents the same still-unreleased capability); do not invent a new released
   section. _Confirm._

## Out of scope
- Any change to conversion behavior, supported formats, or the `sbom.Inventory` model.
- Renaming the `enrich`/`scan` commands or the `pkg/sbom` package (the *verb* "convert" in their
  help text stays — it describes the action, not the command).
