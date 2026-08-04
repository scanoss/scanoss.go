// SPDX-License-Identifier: MIT
/*
 * Copyright (c) 2026, SCANOSS
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in
 * all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
 * THE SOFTWARE.
 */

package cmd

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/scanoss/scanoss.go/internal/config"
	"github.com/scanoss/scanoss.go/internal/output"
	"github.com/scanoss/scanoss.go/pkg/sbom"
	"github.com/spf13/cobra"
)

var sbomCmd = &cobra.Command{
	Use:   "sbom <input>",
	Short: "Produce an SBOM from a scan result or convert between SBOM formats",
	Long: `Produce an SBOM from an existing scan result or convert one SBOM format to another,
offline.

The input format is detected from the file content (a scanoss raw result, a CycloneDX
document, or an SPDX document); the target format is chosen with --format. Conversion is
best-effort: data a target format cannot represent is dropped (for example, SPDX 2.3 has
no vulnerability model, so vulnerabilities are omitted when converting to spdx).

Examples:
  # SPDX -> CycloneDX
  scanoss-cli sbom bom.spdx.json --format cyclonedx --output bom.cdx.json

  # CycloneDX -> SPDX
  scanoss-cli sbom bom.cdx.json --format spdx --output bom.spdx.json

  # scanoss raw result -> CycloneDX or SPDX
  scanoss-cli sbom result.json --format cyclonedx --output bom.cdx.json
  scanoss-cli sbom result.json --format spdx --output bom.spdx.json`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSbom,
}

func init() {
	rootCmd.AddCommand(sbomCmd)
	sbomCmd.Flags().StringP("format", "f", "", "Target format: cyclonedx or spdx")
	sbomCmd.Flags().StringP("output", "o", "", "Output file (empty = stdout)")
}

func runSbom(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return usageError(cmd, "an input file is required")
	}

	// Validate the target up front (raw/plain are not SBOM targets).
	target, _ := cmd.Flags().GetString("format")
	switch target {
	case config.FormatCycloneDX, config.FormatSPDX:
	case "":
		return fmt.Errorf("--format is required (cyclonedx or spdx)")
	default:
		return fmt.Errorf("invalid target format %q: must be cyclonedx or spdx", target)
	}

	data, err := os.ReadFile(args[0])
	if err != nil {
		return fmt.Errorf("error reading input: %w", err)
	}

	inv, _, err := identifyAndParse(data) // input format is detected; the target is --format
	if err != nil {
		return err
	}

	warnDroppedLayers(target, inv)

	out, err := sbom.Generate(inv, sbom.Format(target))
	if err != nil {
		return fmt.Errorf("error generating %s output: %w", target, err)
	}

	outputFile, _ := cmd.Flags().GetString("output")
	writer, err := output.NewWriter(outputFile)
	if err != nil {
		return fmt.Errorf("error creating output writer: %w", err)
	}
	defer func() { _ = writer.Close() }()
	return writer.Write(out)
}

// warnDroppedLayers warns, once per layer, when the target format cannot represent data
// present in the inventory. SPDX 2.3 has no vulnerability model, so vulnerabilities are
// dropped when converting to spdx.
func warnDroppedLayers(target string, inv sbom.Inventory) {
	if target == config.FormatSPDX && len(inv.Vulnerabilities) > 0 {
		slog.Warn("spdx cannot represent vulnerabilities; omitted from the SPDX document",
			"dropped", len(inv.Vulnerabilities))
	}
}

// identifyAndParse detects the input format from content and parses it into a neutral Inventory,
// returning the detected format (raw/cyclonedx/spdx). It is the shared input reader for the `sbom`
// and `enrich` commands, so both agree on what each shape means. CycloneDX and SPDX are recognized
// by their unambiguous markers; anything else is treated as the scanoss raw inventory (the `scan`
// raw output envelope) and parsed with sbom.ParseRaw. A verbatim v3 scan result — whose
// `components` are an object keyed by url_hash, not the inventory's array — is not an accepted
// input and fails to parse (that raw API shape belongs to the SDK, not to these commands).
func identifyAndParse(data []byte) (sbom.Inventory, string, error) {
	var probe struct {
		BOMFormat   string `json:"bomFormat"`
		SPDXVersion string `json:"spdxVersion"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return sbom.Inventory{}, "", fmt.Errorf("input is not valid JSON: %w", err)
	}
	switch {
	case probe.BOMFormat == "CycloneDX":
		inv, err := sbom.ParseCycloneDX(data)
		return inv, config.FormatCycloneDX, err
	case probe.SPDXVersion != "":
		inv, err := sbom.ParseSPDX(data)
		return inv, config.FormatSPDX, err
	default:
		inv, err := sbom.ParseRaw(data)
		if err != nil {
			return sbom.Inventory{}, "", fmt.Errorf("unrecognized input: not a scanoss raw inventory, CycloneDX, or SPDX document: %w", err)
		}
		return inv, config.FormatRaw, nil
	}
}
