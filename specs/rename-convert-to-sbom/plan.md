# Implementation Plan: rename `convert` → `sbom`

**Spec:** `./spec.md` · **Issue:** [#11](https://github.com/scanoss/scanoss.go/issues/11)

## Approach
A pure rename. No logic moves; `sbom.Inventory`, the readers/writers, `scansource`, and the
conversion flow are untouched. We rename the command, its file, its identifiers, its tests, and
every command-name reference in the docs. No `convert` alias (spec Open decision 1).

## Touch points

### `cmd/convert.go → cmd/sbom.go`
- `git mv cmd/convert.go cmd/sbom.go`.
- `var convertCmd → var sbomCmd`; `Use: "convert <input>" → "sbom <input>"`; `RunE: runConvert →
  runSbom`; `func runConvert → func runSbom`.
- Update the `Short`/`Long` help text and examples to `scanoss sbom …`.
- Leave the input-format consts (`inputRaw`/`inputCycloneDX`/`inputSPDX`), `identifyFormat`,
  `inventoryFromInput`, and `warnDroppedLayers` as-is (names are not command-specific).

### `cmd/convert_test.go → cmd/sbom_test.go`
- `git mv`.
- `runConvertTest → runSbomTest`; `TestConvert_* → TestSbom_*`; `TestIdentifyFormat` stays (it
  tests `identifyFormat`, not the command name). Update any in-test comments that say "convert
  command".

### Docs
- `README.md` — Commands table row `| \`convert <input>\` | … |` → `| \`sbom <input>\` | Produce
  or convert an SBOM/result between formats offline (cyclonedx/spdx). |`.
- `CLIENT_HELP.md` — the TOC anchor `[Convert (\`convert\`)](#convert-convert)` →
  `[SBOM (\`sbom\`)](#sbom-sbom)`; the `## Convert (\`convert\`)` heading → `## SBOM (\`sbom\`)`;
  the three `scanoss convert …` examples → `scanoss sbom …`. (The prose verb "convert"/"converting"
  can stay.)
- `CHANGELOG.md` — rename the `**\`convert\`**` entry to `**\`sbom\`**` and its example command
  (spec Open decision 2: rename in place, no new released section).

## Reuse / non-goals
- No change to `pkg/sbom`, `scansource`, `pkg/output`, or the conversion logic.
- The verb "convert" inside `enrich`'s help/comments stays — it describes the action, not the
  command.

## Testing
- The renamed `cmd/sbom_test.go` covers the same cases (identify table, each direction,
  vulnerability-drop warning, invalid `--format`). They must pass unchanged in substance.
- Smoke: `scanoss sbom result.json --format cyclonedx` works; `scanoss convert …` returns
  "unknown command".
- `make check` + `go test ./... -race` clean.

## Risks
- **Stale references.** Grep for `convert` after the rename to catch any missed command-name
  usage (distinguish from the legitimate verb). A `grep -rn "scanoss convert\|convertCmd\|runConvert"`
  should return nothing.
