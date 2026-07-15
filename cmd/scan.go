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
	"path/filepath"
	"sync"

	"github.com/scanoss/scanoss.go/internal/config"
	"github.com/scanoss/scanoss.go/pkg/filter"
	"github.com/scanoss/scanoss.go/pkg/output"
	"github.com/scanoss/scanoss.go/pkg/sbom"
	"github.com/scanoss/scanoss.go/pkg/scanoss"
	"github.com/scanoss/scanoss.go/pkg/scanpipeline"
	"github.com/scanoss/scanoss.go/pkg/settings"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// scanProgress adapts scanoss.Progress updates to terminal progress bars. The
// SDK may call it concurrently (parallel uploads), so it is mutex-guarded.
type scanProgress struct {
	mu         sync.Mutex
	fp         *progressbar.ProgressBar
	upload     *progressbar.ProgressBar
	server     *progressbar.ProgressBar
	fpDone     bool
	uploaded   bool
	serverDone bool
	enrich     *mpb.Progress       // multi-bar container for the concurrent enrichment layers
	layerBars  map[string]*mpb.Bar // one live bar per decoration service, keyed by Service name
}

// fingerprint renders fingerprinting progress. The pipeline calls it before the scan, so it is
// the first phase; the scan and layer phases below arrive later via fn.
func (s *scanProgress) fingerprint(done, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fp == nil {
		s.fp = newPhaseBar(total, "Fingerprinting")
	}
	_ = s.fp.Set(done)
}

// fn renders the SDK's per-service progress: the scan (upload chunks, then server phase) and
// each enrichment layer (Unit "purls"), keyed by Service. It runs the phases in order,
// finalizing each bar as the next begins.
func (s *scanProgress) fn(p scanoss.Progress) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch p.Unit {
	case "chunks":
		s.finishFingerprintLocked()
		if s.upload == nil {
			s.upload = newPhaseBar(p.Total, "Uploading WFP")
		}
		_ = s.upload.Set(p.Done)
	case "phase":
		s.finishFingerprintLocked()
		s.finishUploadLocked()
		if s.server == nil {
			s.server = newPhaseBar(p.Total, "Server processing")
		}
		_ = s.server.Set(p.Done)
	case "purls":
		// Enrichment layers run concurrently — render one live bar per layer via mpb.
		s.finishFingerprintLocked()
		s.finishUploadLocked()
		s.finishServerLocked()
		if p.Service == "" {
			return
		}
		if s.enrich == nil {
			fmt.Fprintln(os.Stderr, "Enriching components")
			s.enrich = mpb.New(mpb.WithOutput(os.Stderr), mpb.WithWidth(40))
			s.layerBars = make(map[string]*mpb.Bar)
		}
		bar, ok := s.layerBars[p.Service]
		if !ok {
			// Match the schollz phase-bar look: "<name> <pct>% |████…| (done/total)".
			bar = s.enrich.New(int64(p.Total),
				mpb.BarStyle().Lbound(" |").Filler("█").Tip("█").Padding(" ").Rbound("|"),
				mpb.PrependDecorators(
					decor.Name(fmt.Sprintf("  %-24s ", layerLabel(p.Service))),
					decor.NewPercentage("%d"), // percentageType already appends "%"
				),
			)
			s.layerBars[p.Service] = bar
		}
		bar.SetCurrent(int64(p.Done))
	}
}

// finishUpload finalizes the upload bar exactly once so later output starts on a
// clean line. Called from the scan-id notify (which fires after the upload) and as
// a fallback when the server phase begins.
func (s *scanProgress) finishUpload() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finishUploadLocked()
}

// finish finalizes all progress rendering once the pipeline returns: the sequential schollz
// phase bars and the mpb enrichment container (waiting for its render goroutine to flush).
func (s *scanProgress) finish() {
	s.mu.Lock()
	s.finishFingerprintLocked()
	s.finishUploadLocked()
	s.finishServerLocked()
	enrich, bars := s.enrich, s.layerBars
	s.mu.Unlock()

	if enrich == nil {
		return
	}
	for _, b := range bars {
		if !b.Completed() {
			b.Abort(false) // a layer that errored never reaches total; stop its bar so Wait returns
		}
	}
	enrich.Wait()
}

func (s *scanProgress) finishFingerprintLocked() {
	if s.fp != nil && !s.fpDone {
		_ = s.fp.Finish()
		fmt.Fprintln(os.Stderr)
		s.fpDone = true
	}
}

func (s *scanProgress) finishUploadLocked() {
	if s.upload != nil && !s.uploaded {
		_ = s.upload.Finish()
		fmt.Fprintln(os.Stderr)
		s.uploaded = true
	}
}

func (s *scanProgress) finishServerLocked() {
	if s.server != nil && !s.serverDone {
		_ = s.server.Finish()
		fmt.Fprintln(os.Stderr)
		s.serverDone = true
	}
}

// layerLabel maps a decoration service name to a human phrase for the "Gathered …" progress line.
func layerLabel(service string) string {
	switch service {
	case "licenses":
		return "licenses"
	case "vulnerabilities":
		return "vulnerabilities"
	case "cryptography.algorithms":
		return "cryptographic algorithms"
	case "geoprovenance.origin":
		return "provenance"
	case "dependencies":
		return "declared dependencies"
	default:
		return service
	}
}

// formatLayers lists which --include layers each output format can render. deps and licenses are
// representable everywhere (as components / package licenses); vulnerabilities in raw + cyclonedx;
// cryptography and geoprovenance only in raw.
var formatLayers = map[string]map[scanpipeline.Layer]bool{
	config.FormatRaw:       {scanpipeline.LayerDeps: true, scanpipeline.LayerLicenses: true, scanpipeline.LayerVulns: true, scanpipeline.LayerCrypto: true, scanpipeline.LayerGeo: true},
	config.FormatCycloneDX: {scanpipeline.LayerDeps: true, scanpipeline.LayerLicenses: true, scanpipeline.LayerVulns: true},
	config.FormatSPDX:      {scanpipeline.LayerDeps: true, scanpipeline.LayerLicenses: true},
}

// layerName is the readable name of a layer, for warnings.
func layerName(l scanpipeline.Layer) string {
	switch l {
	case scanpipeline.LayerVulns:
		return "vulnerabilities"
	case scanpipeline.LayerCrypto:
		return "cryptography"
	case scanpipeline.LayerGeo:
		return "geoprovenance"
	case scanpipeline.LayerDeps:
		return "dependencies"
	default:
		return string(l) // licenses
	}
}

// unsupportedLayers returns the requested layers the format cannot render, in a stable order.
func unsupportedLayers(format string, layers scanpipeline.Set) []scanpipeline.Layer {
	caps := formatLayers[format]
	var out []scanpipeline.Layer
	for _, l := range []scanpipeline.Layer{scanpipeline.LayerDeps, scanpipeline.LayerLicenses, scanpipeline.LayerVulns, scanpipeline.LayerCrypto, scanpipeline.LayerGeo} {
		if layers.Has(l) && !caps[l] {
			out = append(out, l)
		}
	}
	return out
}

// effectiveLayers returns the requested layers the format can actually render
// (requested ∩ capabilities) — the set the fused scan command gathers, so it never fetches a
// layer this format would drop.
func effectiveLayers(format string, layers scanpipeline.Set) scanpipeline.Set {
	caps := formatLayers[format]
	eff := make(scanpipeline.Set, len(layers))
	for l := range layers {
		if caps[l] {
			eff[l] = true
		}
	}
	return eff
}

// reportSkippedLayers tells the user, up front, which requested layers the chosen format cannot
// represent and so will not be gathered.
func reportSkippedLayers(format string, layers scanpipeline.Set) {
	for _, l := range unsupportedLayers(format, layers) {
		fmt.Fprintf(os.Stderr, "Skipping %s: not supported by the %s format\n", layerName(l), format)
	}
}

func newPhaseBar(total int, desc string) *progressbar.ProgressBar {
	return progressbar.NewOptions(total,
		progressbar.OptionSetDescription(desc),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(40),
		progressbar.OptionSetRenderBlankState(true),
	)
}

// buildResultsCommand constructs the resume command printed for recovery.
func buildResultsCommand(scanID, apiURL, apiKey string) string {
	cmd := fmt.Sprintf("  scanoss results %s", scanID)
	if apiURL != config.DefaultAPIURL {
		cmd += fmt.Sprintf(" --api-url %s", apiURL)
	}
	if apiKey != "" {
		cmd += " --api-key ***" // masked — never echo the real key to the terminal/logs
	}
	return cmd
}

// printErrorSummary prints a summary if any files failed to fingerprint.
func printErrorSummary(processErrors int) {
	if processErrors > 0 {
		fmt.Fprintf(os.Stderr, "Completed with %d processing errors\n", processErrors)
	}
}

// validateOutputFormat checks the --format flag up front so an invalid value fails
// fast, before any fingerprinting or upload.
func validateOutputFormat(cmd *cobra.Command) error {
	format, _ := cmd.Flags().GetString("format")
	switch format {
	case config.FormatRaw, config.FormatSPDX, config.FormatCycloneDX:
		return nil
	default:
		return fmt.Errorf("invalid output format %q: must be raw, spdx, or cyclonedx", format)
	}
}

var scanCmd = &cobra.Command{
	Use:   "scan <path>",
	Short: "Scan a file or folder and send results to the SCANOSS API",
	Long: `The scan command scans a single file or recursively scans a folder, generates
WFP fingerprints, uploads them to the SCANOSS v3 batch scan API in parallel
chunks, and polls until the scan completes. To scan a pre-generated WFP file
instead, use the "wfp" subcommand.

The scan id is printed; if the scan is interrupted you can resume
it later with "scanoss results <scan-id>".

Examples:
  # Scan a folder
  scanoss scan ./my-project

  # Scan a single file
  scanoss scan ./src/main.c

  # Scan with custom API URL and key
  scanoss scan ./my-project --api-url https://api.scanoss.com --api-key TOKEN

  # Scan a pre-generated WFP file
  scanoss scan wfp fingerprints.wfp

  # Save WFP fingerprints before sending to the API
  scanoss scan ./my-project --save-wfp fingerprints.wfp`,
	Args: cobra.MaximumNArgs(1),
	RunE: runScan,
}

var scanWFPCmd = &cobra.Command{
	Use:   "wfp <wfp-path>",
	Short: "Scan a pre-generated WFP fingerprint file",
	Long: `The wfp subcommand uploads an already-assembled WFP fingerprint file to the
SCANOSS v3 batch scan API and polls until the scan completes. No fingerprinting
is performed; the file is sent as-is.

Examples:
  # Scan a WFP file produced earlier (e.g. with --save-wfp or the wfp command)
  scanoss scan wfp fingerprints.wfp

  # Scan a WFP file against a premium endpoint
  scanoss scan wfp fingerprints.wfp --api-key TOKEN`,
	Args: cobra.MaximumNArgs(1),
	RunE: runScanWFP,
}

func init() {
	rootCmd.AddCommand(scanCmd)
	scanCmd.AddCommand(scanWFPCmd)

	// Shared flags (persistent → inherited by the wfp subcommand).
	scanCmd.PersistentFlags().String("api-url", config.DefaultAPIURL, "SCANOSS API URL")
	scanCmd.PersistentFlags().String("api-key", "", "API authentication token")
	scanCmd.PersistentFlags().StringP("output", "o", "", "Output file (empty = stdout)")
	scanCmd.PersistentFlags().StringP("format", "f", config.DefaultFormat, "Result output format: raw, spdx, cyclonedx")
	scanCmd.PersistentFlags().String("settings", "", "Path to settings file (scanoss.json/settings.json)")
	scanCmd.PersistentFlags().Int("chunk-size", scanoss.DefaultScanChunkBytes, "WFP upload chunk size in bytes")
	scanCmd.PersistentFlags().Duration("poll-interval", scanoss.DefaultScanPollInterval, "How often to poll for scan status")
	scanCmd.PersistentFlags().Bool("ignore-cert-errors", false, "Ignore TLS certificate errors (insecure)")
	scanCmd.PersistentFlags().StringSlice("include", nil, "Output layers to gather (comma-separated): deps, vulns, licenses, crypto, geo")

	// Fingerprinting flags (apply to `scan <path>` only).
	scanCmd.Flags().IntP("threads", "t", config.DefaultThreads, "Number of parallel fingerprint workers")
	scanCmd.Flags().String("save-wfp", "", "Save WFP fingerprints to file before sending to API")
	scanCmd.Flags().Int64("max-size", 0, "Maximum file size in bytes to scan (0 = unlimited)")
	scanCmd.Flags().Bool("default-filters", true, "Apply the built-in default file filters")
	scanCmd.Flags().Bool("gitignore", true, "Honor .gitignore files when collecting files")
}

// runScan fingerprints a file or folder and scans it. Collection, fingerprinting, the scan, and
// enrichment all happen inside scanpipeline.Run; this command only gathers flags, renders
// progress, and writes the result.
func runScan(cmd *cobra.Command, args []string) error {
	// No path given: show usage instead of a terse arg error or the auth banner.
	if len(args) == 0 {
		return cmd.Help()
	}
	if err := checkAuth(cmd); err != nil {
		return err
	}
	if err := validateOutputFormat(cmd); err != nil {
		return err
	}
	layers, err := scanLayers(cmd)
	if err != nil {
		return err
	}
	outputFormat, _ := cmd.Flags().GetString("format")
	reportSkippedLayers(outputFormat, layers)
	layers = effectiveLayers(outputFormat, layers) // don't gather what this format can't render

	targetPath := args[0]
	if _, err := os.Stat(targetPath); err != nil {
		return fmt.Errorf("error accessing path: %w", err)
	}

	threads, _ := cmd.Flags().GetInt("threads")
	if threads < 1 {
		threads = config.DefaultThreads
	}
	saveWFPFile, _ := cmd.Flags().GetString("save-wfp")
	settingsFlag, _ := cmd.Flags().GetString("settings")
	maxSize, _ := cmd.Flags().GetInt64("max-size")
	applyDefaultFilters, _ := cmd.Flags().GetBool("default-filters")
	applyGitignore, _ := cmd.Flags().GetBool("gitignore")

	// Settings drive file filtering. Of the BOM rules, only bom.remove is applied (SDK-side,
	// post-scan, via WithBOM); identify/ignore/replace are not applied.
	scanSettings, err := settings.Resolve(settingsFlag, targetPath)
	if err != nil {
		return fmt.Errorf("error loading settings: %w", err)
	}

	prog := &scanProgress{}
	client := buildScanClient(cmd, prog)

	ctx, cancel := createCancellableContext()
	defer cancel()

	result, err := scanpipeline.Run(ctx, scanpipeline.Options{
		Client:     client,
		Layers:     layers,
		SourcePath: targetPath,
		Threads:    threads,
		Filter: filter.Options{
			MaxSize:   maxSize,
			Defaults:  applyDefaultFilters,
			GitIgnore: applyGitignore,
			Settings:  scanSettings.ScanFilter(),
		},
		ScanOptions: scanTuning(cmd, scanSettings),
		OnCollect: func(skipped int) {
			if skipped > 0 {
				fmt.Fprintf(os.Stderr, "Filtered %d files\n", skipped)
			}
		},
		OnFingerprint: prog.fingerprint,
	})
	prog.finish()
	if err != nil {
		return renderAPIError(fmt.Errorf("scan failed: %w", err))
	}
	if saveWFPFile != "" && len(result.WFP) > 0 {
		if err := os.WriteFile(saveWFPFile, result.WFP, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write WFP file: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "WFP fingerprints saved to: %s\n", saveWFPFile)
		}
	}

	if err := emitInventory(cmd, result.Inventory, targetPath); err != nil {
		return err
	}
	printErrorSummary(result.ProcessErrors)
	return nil
}

// runScanWFP scans a pre-generated WFP file directly (no fingerprinting). A bare WFP has no
// source tree, so the deps layer cannot be sourced; it uses the lower-level scanpipeline.Build.
func runScanWFP(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	if err := checkAuth(cmd); err != nil {
		return err
	}
	if err := validateOutputFormat(cmd); err != nil {
		return err
	}
	layers, err := scanLayers(cmd)
	if err != nil {
		return err
	}
	if layers.Has(scanpipeline.LayerDeps) {
		fmt.Fprintln(os.Stderr, "Warning: the deps layer needs a source tree; ignored when scanning a WFP file")
	}
	outputFormat, _ := cmd.Flags().GetString("format")
	reportSkippedLayers(outputFormat, layers)
	layers = effectiveLayers(outputFormat, layers) // don't gather what this format can't render

	wfpPath := args[0]
	info, err := os.Stat(wfpPath)
	if err != nil {
		return fmt.Errorf("error accessing WFP file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("%q is a directory; expected a WFP file", wfpPath)
	}
	wfp, err := os.ReadFile(wfpPath)
	if err != nil {
		return fmt.Errorf("error reading WFP file: %w", err)
	}
	if len(wfp) == 0 {
		return fmt.Errorf("WFP file %q is empty", wfpPath)
	}

	settingsFlag, _ := cmd.Flags().GetString("settings")
	scanSettings, err := settings.Resolve(settingsFlag, filepath.Dir(wfpPath))
	if err != nil {
		return fmt.Errorf("error loading settings: %w", err)
	}

	prog := &scanProgress{}
	client := buildScanClient(cmd, prog)

	ctx, cancel := createCancellableContext()
	defer cancel()

	res, err := client.Scan.WFP(ctx, wfp, scanTuning(cmd, scanSettings)...)
	if err != nil {
		return renderAPIError(fmt.Errorf("scan failed: %w", err))
	}
	if res.Result == nil {
		return fmt.Errorf("scan completed without a result")
	}

	inv, err := scanpipeline.Build(ctx, client, res.Result, layers, nil)
	prog.finish()
	if err != nil {
		return err
	}
	return emitInventory(cmd, inv, wfpPath)
}

// buildScanClient constructs the SDK client from the shared scan flags, wiring the progress
// renderer (scan + per-layer, keyed by Service) and the scan-id notify hook.
func buildScanClient(cmd *cobra.Command, prog *scanProgress) *scanoss.Client {
	apiURL, _ := cmd.Flags().GetString("api-url")
	apiKey, _ := cmd.Flags().GetString("api-key")
	ignoreCertErrors, _ := cmd.Flags().GetBool("ignore-cert-errors")

	opts := []scanoss.Option{
		scanoss.WithAPIURL(apiURL),
		scanoss.WithAPIKey(apiKey),
		scanoss.WithProgress(prog.fn),
		scanoss.WithScanIDNotify(func(id string) {
			prog.finishUpload()
			fmt.Fprintf(os.Stderr, "\nScan id: %s\n", id)
			fmt.Fprintf(os.Stderr, "If interrupted, resume with:\n%s\n\n", buildResultsCommand(id, apiURL, apiKey))
		}),
	}
	if ignoreCertErrors {
		slog.Warn("ignoring TLS certificate errors (insecure)")
		opts = append(opts, scanoss.WithInsecureTLS(true))
	}
	return scanoss.New(opts...)
}

// scanTuning builds the per-scan options (chunk size, poll interval, and bom.remove) from the
// flags and settings.
func scanTuning(cmd *cobra.Command, scanSettings *settings.Settings) []scanoss.ScanOption {
	chunkSize, _ := cmd.Flags().GetInt("chunk-size")
	pollInterval, _ := cmd.Flags().GetDuration("poll-interval")
	if chunkSize < 1024 {
		chunkSize = scanoss.DefaultScanChunkBytes
	}
	scanOpts := []scanoss.ScanOption{scanoss.WithChunkBytes(chunkSize), scanoss.WithPollInterval(pollInterval)}
	if scanSettings != nil && scanSettings.HasBOM() {
		scanOpts = append(scanOpts, scanoss.WithBOM(&scanSettings.BOM))
	}
	return scanOpts
}

// emitInventory renders inv in the --format and writes it to the --output target. scanPath names
// the SBOM project.
func emitInventory(cmd *cobra.Command, inv sbom.Inventory, scanPath string) error {
	outputFile, _ := cmd.Flags().GetString("output")
	outputFormat, _ := cmd.Flags().GetString("format")

	writer, err := output.NewWriter(outputFile)
	if err != nil {
		return fmt.Errorf("error creating output writer: %w", err)
	}
	defer func() { _ = writer.Close() }()

	projectName := filepath.Base(scanPath)
	if projectName == "." || projectName == "/" {
		projectName = "" // let the sbom module apply its default
	}
	out, err := renderInventory(inv, outputFormat, projectName)
	if err != nil {
		return err
	}
	if err := writer.Write(out); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing: %v\n", err)
	} else if outputFile != "" {
		fmt.Fprintf(os.Stderr, "Results written to %s\n", outputFile)
	}
	return nil
}

// scanLayers reads and validates the --include flag into a scanpipeline layer set. Gathering
// is driven by this set — never by the output format (FR-001).
func scanLayers(cmd *cobra.Command) (scanpipeline.Set, error) {
	values, _ := cmd.Flags().GetStringSlice("include")
	return scanpipeline.ParseLayers(values)
}

// rawSchemaVersion is the version of the raw inventory document. Bump on a breaking shape change.
const rawSchemaVersion = "1.0"

// rawDoc is the raw output document: the inventory wrapped in a versioned envelope. The
// embedded Inventory promotes its `components` / `vulnerabilities` keys to the top level, so the
// document is `{schema_version, metadata, components, vulnerabilities}` — the interchange
// contract for a future scan → enrich → sbom pipe.
type rawDoc struct {
	SchemaVersion string      `json:"schema_version"`
	Metadata      rawMetadata `json:"metadata"`
	sbom.Inventory
}

type rawMetadata struct {
	Tool        string `json:"tool"`
	ToolVersion string `json:"tool_version"`
	Project     string `json:"project,omitempty"`
}

// renderInventory renders a gathered inventory in the requested format. raw wraps the inventory
// in the versioned envelope as JSON; cyclonedx/spdx project it to an SBOM, dropping what they
// cannot represent.
func renderInventory(inv sbom.Inventory, format, projectName string) (string, error) {
	if format == config.FormatRaw {
		doc := rawDoc{
			SchemaVersion: rawSchemaVersion,
			Metadata:      rawMetadata{Tool: config.AppName, ToolVersion: config.AppVersion, Project: projectName},
			Inventory:     inv,
		}
		raw, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return "", fmt.Errorf("error encoding results: %w", err)
		}
		return string(raw), nil
	}
	out, err := sbom.Generate(inv, sbom.Format(format), sbom.WithProjectName(projectName))
	if err != nil {
		return "", fmt.Errorf("error generating %s output: %w", format, err)
	}
	return out, nil
}
