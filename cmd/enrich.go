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
	"os"

	"github.com/scanoss/scanoss.go/internal/config"
	"github.com/scanoss/scanoss.go/pkg/sbom"
	"github.com/scanoss/scanoss.go/pkg/scanpipeline"
	"github.com/spf13/cobra"
)

var enrichCmd = &cobra.Command{
	Use:   "enrich <input>",
	Short: "Decorate an existing inventory/SBOM with purl-keyed layers",
	Long: `The enrich command takes an existing inventory or SBOM — a scanoss raw inventory
(the scan raw output), a CycloneDX document, or an SPDX document — and decorates its
components with the requested purl-keyed layers (vulns, licenses, crypto, geo) through
the SCANOSS API, then renders the result in the chosen format.

It needs only the inventory: no source tree, no fingerprinting, no re-scan. Because it is
keyed purely by PURL, it is re-runnable (e.g. weekly) to refresh the layers against the
same file. The output format defaults to the input's format; use --format to convert in the
same pass. A layer the output format cannot represent is skipped with an up-front notice.

Examples:
  # Refresh vulns/licenses/crypto on a raw inventory (raw in, raw out)
  scanoss enrich inv.json --include vulns,licenses,crypto > enriched.json

  # Enrich an SPDX document (spdx in, spdx out)
  scanoss enrich sbom.spdx.json --include licenses > enriched.spdx.json

  # Enrich a CycloneDX document and convert to SPDX in one pass
  scanoss enrich sbom.cdx.json --include licenses --format spdx > enriched.spdx.json`,
	Args: cobra.MaximumNArgs(1),
	RunE: runEnrich,
}

func init() {
	rootCmd.AddCommand(enrichCmd)
	enrichCmd.Flags().String("api-url", config.DefaultAPIURL, "SCANOSS API URL")
	enrichCmd.Flags().String("api-key", "", "API authentication token")
	enrichCmd.Flags().StringP("output", "o", "", "Output file (empty = stdout)")
	enrichCmd.Flags().StringP("format", "f", "", "Output format: raw, spdx, cyclonedx (default: same as input)")
	enrichCmd.Flags().Bool("ignore-cert-errors", false, "Ignore TLS certificate errors (insecure)")
	enrichCmd.Flags().StringSlice("include", nil, "Layers to gather (comma-separated): vulns, licenses, crypto, geo")
}

// runEnrich parses an inventory/SBOM, enriches its components with the requested purl-layers
// through the decoration pipeline, and renders the result. No scanning is involved: enrichment is
// the scan pipeline's format-blind tail stage (scanpipeline.Enrich) run on an inventory the
// command parsed itself.
func runEnrich(cmd *cobra.Command, args []string) error {
	// No input given: show usage instead of a terse arg error.
	if len(args) == 0 {
		return cmd.Help()
	}
	if err := checkAuth(cmd); err != nil {
		return err
	}

	layers, err := scanLayers(cmd)
	if err != nil {
		return err
	}
	// deps is not a valid enrich layer: dependency analysis needs a manifest/source tree and
	// cannot be derived from a flat components list. Reject it up front rather than silently drop.
	if layers.Has(scanpipeline.LayerDeps) {
		return fmt.Errorf("--include deps is not supported by enrich: dependencies cannot be analysed over a components list (valid: vulns, licenses, crypto, geo)")
	}

	data, err := os.ReadFile(args[0])
	if err != nil {
		return fmt.Errorf("error reading input: %w", err)
	}
	inv, inputFormat, err := identifyAndParse(data)
	if err != nil {
		return err
	}

	// Output format defaults to the input's; --format overrides (converting in the same pass).
	outputFormat, _ := cmd.Flags().GetString("format")
	if outputFormat == "" {
		outputFormat = inputFormat
	}
	switch outputFormat {
	case config.FormatRaw, config.FormatSPDX, config.FormatCycloneDX:
	default:
		return fmt.Errorf("invalid output format %q: must be raw, spdx, or cyclonedx", outputFormat)
	}
	_ = cmd.Flags().Set("format", outputFormat) // let emitInventory read the resolved format

	// Narrow to what the output format can render, reporting each skipped layer up front — the
	// same capability logic the scan command uses, so enrich never gathers a layer it would drop.
	reportSkippedLayers(outputFormat, layers)
	layers = effectiveLayers(outputFormat, layers)

	prog := &scanProgress{}
	client := buildScanClient(cmd, prog)

	ctx, cancel := createCancellableContext()
	defer cancel()

	scanpipeline.Enrich(ctx, client, &inv, layers)
	prog.finish()

	return emitInventory(cmd, inv, args[0])
}

// identifyAndParse detects the input format from content and parses it into a neutral Inventory,
// returning the detected format so the command can default --format to it. CycloneDX and SPDX are
// recognized by their unambiguous markers; anything else is treated as the scanoss raw inventory
// (the scan raw output) and unmarshalled directly. A verbatim v3 scan result — whose components
// are an object keyed by url_hash, not the inventory's array — fails to unmarshal here and errors,
// which is intended: it is not an accepted input.
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
