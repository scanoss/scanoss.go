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
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/scanoss/scanoss.go/internal/cliconfig"
	"github.com/scanoss/scanoss.go/internal/config"
	"github.com/scanoss/scanoss.go/pkg/filter"
	"github.com/scanoss/scanoss.go/pkg/output"
	"github.com/scanoss/scanoss.go/pkg/sbom"
	"github.com/scanoss/scanoss.go/pkg/scanoss"
	"github.com/scanoss/scanoss.go/pkg/scanpipeline"
	"github.com/scanoss/scanoss.go/pkg/settings"
	"github.com/spf13/cobra"
	"github.com/vbauerster/mpb/v8"
)

// scanProgress adapts scanoss.Progress updates to terminal progress bars. Every phase — the scan
// (fingerprint, upload, server) and each purl-keyed service (declared-dependency resolution and
// the enrichment layers) — is a bar in one shared mpb container, so phases that run concurrently
// render side by side. The SDK may call it from several goroutines, so it is mutex-guarded.
type scanProgress struct {
	mu   sync.Mutex
	p    *mpb.Progress       // shared multi-bar container (lazily created on first update)
	bars map[string]*mpb.Bar // one live bar per phase/service, keyed by phase key or Service name
}

// fingerprint renders fingerprinting progress. The pipeline calls it as fingerprinting proceeds;
// it is one bar among the phases, and can render alongside the dependency phase running in parallel.
func (s *scanProgress) fingerprint(done, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.barLocked("fingerprint", "Fingerprinting", total).SetCurrent(int64(done))
}

// dependencies renders dependency-manifest parsing progress as a bar in the shared container,
// alongside the scan phases. Parsing is local (no API), so it fills quickly.
func (s *scanProgress) dependencies(done, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.barLocked("deps", "Parsing dependencies", total).SetCurrent(int64(done))
}

// fn renders the SDK's per-service progress in the shared container: the scan (upload chunks,
// then server phase) and every purl-keyed service (declared-dependency resolution and each
// enrichment layer, Unit "purls"), keyed by Service. Each bar fills independently, so the scan
// and dependency phases render side by side while they run in parallel.
func (s *scanProgress) fn(p scanoss.Progress) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch p.Unit {
	case "chunks":
		s.barLocked("upload", "Uploading WFP", p.Total).SetCurrent(int64(p.Done))
	case "phase":
		s.barLocked("server", "Scanning", p.Total).SetCurrent(int64(p.Done))
	case "purls":
		if p.Service == "" {
			return
		}
		// The enrichment layers start after the scan phase (dependency resolution is part of that
		// phase); separate the two groups with a blank line the first time a layer appears.
		if p.Service != scanoss.ServiceDependencies.Name {
			s.spacerLocked()
		}
		s.barLocked(p.Service, layerLabel(p.Service), p.Total).SetCurrent(int64(p.Done))
	}
}

// initLocked creates the shared container on first use. Caller holds s.mu.
func (s *scanProgress) initLocked() {
	if s.p == nil {
		s.p = newProgress()
		s.bars = make(map[string]*mpb.Bar)
	}
}

// spacerLocked adds a blank line once, separating the scan-phase bars (fingerprint, upload,
// scan, dependency resolution) from the enrichment layer bars below. It is a completed no-op bar,
// so it just occupies an empty line and never blocks Wait. Caller holds s.mu.
func (s *scanProgress) spacerLocked() {
	s.initLocked()
	if _, ok := s.bars["_spacer"]; ok {
		return
	}
	bar := s.p.New(1, mpb.NopStyle())
	bar.SetCurrent(1)
	s.bars["_spacer"] = bar
}

// barLocked returns the bar for key, creating it in the shared container on first use. Caller
// holds s.mu.
func (s *scanProgress) barLocked(key, label string, total int) *mpb.Bar {
	s.initLocked()
	bar, ok := s.bars[key]
	if !ok {
		bar = addBar(s.p, total, label)
		s.bars[key] = bar
	}
	return bar
}

// writeLine prints a line above the running bars (e.g. the scan-id notice) without corrupting the
// bar area — mpb's *Progress is an io.Writer that interleaves the line at the next refresh.
func (s *scanProgress) writeLine(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initLocked()
	_, _ = fmt.Fprintln(s.p, msg)
}

// finish finalizes all progress once the pipeline returns: any bar that never reached its total
// (e.g. a service that errored) is aborted so the container's render goroutine can flush and stop.
func (s *scanProgress) finish() {
	s.mu.Lock()
	p, bars := s.p, s.bars
	s.mu.Unlock()

	if p == nil {
		return
	}
	for _, b := range bars {
		if !b.Completed() {
			b.Abort(false)
		}
	}
	p.Wait()
}

// layerLabel maps a decoration service name to a human phrase for the "Gathered …" progress line.
func layerLabel(service string) string {
	switch service {
	case "licenses":
		return "Licenses"
	case "vulnerabilities":
		return "Vulnerabilities"
	case "cryptography.algorithms":
		return "Cryptography"
	case "geoprovenance.origin":
		return "Geoprovenance"
	case "dependencies":
		return "Resolving dependencies"
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
// represent and so will not be gathered — grouped into a single warning rather than one line per
// layer.
func reportSkippedLayers(format string, layers scanpipeline.Set) {
	skipped := unsupportedLayers(format, layers)
	if len(skipped) == 0 {
		return
	}
	names := make([]string, len(skipped))
	for i, l := range skipped {
		names[i] = layerName(l)
	}
	warnf("%s can't render %s — skipped", format, strings.Join(names, ", "))
}

// buildResultsCommand constructs the resume command printed for recovery.
func buildResultsCommand(scanID, apiURL, apiKey string) string {
	cmd := fmt.Sprintf("  scanoss-cli results %s", scanID)
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
		warnf("Completed with %d processing errors", processErrors)
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
it later with "scanoss-cli results <scan-id>".

Examples:
  # Scan a folder
  scanoss-cli scan ./my-project

  # Scan a single file
  scanoss-cli scan ./src/main.c

  # Scan with custom API URL and key
  scanoss-cli scan ./my-project --api-url https://api.scanoss.com --api-key TOKEN

  # Scan a pre-generated WFP file
  scanoss-cli scan wfp fingerprints.wfp

  # Save WFP fingerprints before sending to the API
  scanoss-cli scan ./my-project --save-wfp fingerprints.wfp`,
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
  scanoss-cli scan wfp fingerprints.wfp

  # Scan a WFP file against a premium endpoint
  scanoss-cli scan wfp fingerprints.wfp --api-key TOKEN`,
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
	client, err := buildScanClient(cmd, prog)
	if err != nil {
		return err
	}

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
				infof("Filtered %d files", skipped)
			}
		},
		OnFingerprint:  prog.fingerprint,
		OnDependencies: prog.dependencies,
	})
	prog.finish()
	if err != nil {
		return renderAPIError(fmt.Errorf("scan failed: %w", err))
	}
	if saveWFPFile != "" && len(result.WFP) > 0 {
		if err := os.WriteFile(saveWFPFile, result.WFP, 0o644); err != nil {
			warnf("failed to write WFP file: %v", err)
		} else {
			okf("WFP fingerprints saved to %s", saveWFPFile)
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
		warnf("the deps layer needs a source tree; ignored when scanning a WFP file")
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
	client, err := buildScanClient(cmd, prog)
	if err != nil {
		return err
	}

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
func buildScanClient(cmd *cobra.Command, prog *scanProgress) (*scanoss.Client, error) {
	api, err := cliconfig.ResolveAPI(cmd.Flags())
	if err != nil {
		return nil, err
	}
	ignoreCertErrors, _ := cmd.Flags().GetBool("ignore-cert-errors")

	opts := []scanoss.Option{
		scanoss.WithAPIURL(api.URL),
		scanoss.WithAPIKey(api.Key),
		scanoss.WithProgress(prog.fn),
		scanoss.WithScanIDNotify(func(id string) {
			prog.writeLine("") // separate the scan-id block from the filter/skip notices above
			prog.writeLine(infoLine("Scan id: %s", id))
			// The resume hint carries the resolved endpoint, so it still works in a
			// shell without the environment variable that produced it.
			prog.writeLine("  If interrupted, resume with:\n  " + buildResultsCommand(id, api.URL, api.Key))
			prog.writeLine("") // separate the resume hint from the progress bars below
		}),
	}
	if ignoreCertErrors {
		slog.Warn("ignoring TLS certificate errors (insecure)")
		opts = append(opts, scanoss.WithInsecureTLS(true))
	}
	return scanoss.New(opts...), nil
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
		okf("Results written to %s", outputFile)
	}
	return nil
}

// scanLayers reads and validates the --include flag into a scanpipeline layer set. Gathering
// is driven by this set — never by the output format (FR-001).
func scanLayers(cmd *cobra.Command) (scanpipeline.Set, error) {
	values, _ := cmd.Flags().GetStringSlice("include")
	return scanpipeline.ParseLayers(values)
}

// renderInventory renders a gathered inventory in the requested format. raw wraps the inventory
// in the versioned envelope (sbom.RawDocument) as JSON; cyclonedx/spdx project it to an SBOM,
// dropping what they cannot represent.
func renderInventory(inv sbom.Inventory, format, projectName string) (string, error) {
	if format == config.FormatRaw {
		doc := sbom.NewRawDocument(inv, sbom.RawMetadata{
			Tool: config.AppName, ToolVersion: config.AppVersion, Project: projectName,
		})
		raw, err := doc.Marshal()
		if err != nil {
			return "", fmt.Errorf("error encoding results: %w", err)
		}
		return raw, nil
	}
	out, err := sbom.Generate(inv, sbom.Format(format), sbom.WithProjectName(projectName))
	if err != nil {
		return "", fmt.Errorf("error generating %s output: %w", format, err)
	}
	return out, nil
}
