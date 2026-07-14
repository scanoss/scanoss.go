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

	scanossapi "github.com/scanoss/scanoss.api-sdk"

	"github.com/scanoss/scanoss.go/internal/config"
	"github.com/scanoss/scanoss.go/pkg/output"
	"github.com/scanoss/scanoss.go/pkg/sbom"
	"github.com/scanoss/scanoss.go/pkg/sbom/scansource"
	"github.com/spf13/cobra"
)

// Input formats recognized by the convert command.
const (
	inputRaw       = "raw"
	inputCycloneDX = "cyclonedx"
	inputSPDX      = "spdx"
)

var convertCmd = &cobra.Command{
	Use:   "convert <input>",
	Short: "Convert an SBOM/scan result between formats",
	Long: `Convert an existing SBOM or scan result between formats.

The input format is detected from the file content (a scanoss raw result, a CycloneDX
document, or an SPDX document); the target format is chosen with --format. Conversion is
best-effort: data a target format cannot represent is dropped (for example, SPDX 2.3 has
no vulnerability model, so vulnerabilities are omitted when converting to spdx).

Examples:
  # SPDX -> CycloneDX
  scanoss convert bom.spdx.json --format cyclonedx --output bom.cdx.json

  # CycloneDX -> SPDX
  scanoss convert bom.cdx.json --format spdx --output bom.spdx.json

  # scanoss raw result -> CycloneDX or SPDX
  scanoss convert result.json --format cyclonedx --output bom.cdx.json
  scanoss convert result.json --format spdx --output bom.spdx.json`,
	Args: cobra.MaximumNArgs(1),
	RunE: runConvert,
}

func init() {
	rootCmd.AddCommand(convertCmd)
	convertCmd.Flags().StringP("format", "f", "", "Target format: cyclonedx or spdx")
	convertCmd.Flags().StringP("output", "o", "", "Output file (empty = stdout)")
}

func runConvert(cmd *cobra.Command, args []string) error {
	// No input given: show usage instead of a terse arg error.
	if len(args) == 0 {
		return cmd.Help()
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

	inputFormat, err := identifyFormat(data)
	if err != nil {
		return err
	}

	inv, err := inventoryFromInput(data, inputFormat)
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

// identifyFormat inspects the input's content and returns its format. It checks each
// format's unambiguous marker; unrecognized input is an error (no guessing, no --from).
func identifyFormat(data []byte) (string, error) {
	var probe struct {
		BOMFormat   string          `json:"bomFormat"`
		SPDXVersion string          `json:"spdxVersion"`
		Components  json.RawMessage `json:"components"`
		Files       json.RawMessage `json:"files"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return "", fmt.Errorf("input is not valid JSON: %w", err)
	}
	switch {
	case probe.BOMFormat == "CycloneDX":
		return inputCycloneDX, nil
	case probe.SPDXVersion != "":
		return inputSPDX, nil
	case probe.Components != nil || probe.Files != nil:
		return inputRaw, nil
	default:
		return "", fmt.Errorf("unrecognized input: not a scanoss result, CycloneDX, or SPDX document")
	}
}

// inventoryFromInput parses the input bytes into a neutral Inventory using the reader for
// the detected format.
func inventoryFromInput(data []byte, format string) (sbom.Inventory, error) {
	switch format {
	case inputCycloneDX:
		return sbom.ParseCycloneDX(data)
	case inputSPDX:
		return sbom.ParseSPDX(data)
	case inputRaw:
		var result scanossapi.ScanResult
		if err := json.Unmarshal(data, &result); err != nil {
			return sbom.Inventory{}, fmt.Errorf("error decoding scanoss result: %w", err)
		}
		return scansource.FromScanResult(&result), nil
	default:
		return sbom.Inventory{}, fmt.Errorf("unsupported input format %q", format)
	}
}
