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
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	scanossapi "github.com/scanoss/scanoss.api-sdk"
	"path/filepath"
	"sync"

	"github.com/scanoss/scanoss.go/internal/config"
	"github.com/scanoss/scanoss.go/pkg/filter"
	"github.com/scanoss/scanoss.go/pkg/output"
	"github.com/scanoss/scanoss.go/pkg/sbom"
	"github.com/scanoss/scanoss.go/pkg/sbom/scansource"
	"github.com/scanoss/scanoss.go/pkg/scanner"
	"github.com/scanoss/scanoss.go/pkg/scanoss"
	"github.com/scanoss/scanoss.go/pkg/settings"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

// scanProgress adapts scanoss.Progress updates to terminal progress bars. The
// SDK may call it concurrently (parallel uploads), so it is mutex-guarded.
type scanProgress struct {
	mu       sync.Mutex
	upload   *progressbar.ProgressBar
	server   *progressbar.ProgressBar
	uploaded bool
}

func (s *scanProgress) fn(p scanoss.Progress) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch p.Unit {
	case "chunks":
		if s.upload == nil {
			s.upload = newPhaseBar(p.Total, "Uploading WFP")
		}
		_ = s.upload.Set(p.Done)
	case "phase":
		s.finishUploadLocked()
		if s.server == nil {
			s.server = newPhaseBar(p.Total, "Server processing")
		}
		_ = s.server.Set(p.Done)
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

func (s *scanProgress) finishUploadLocked() {
	if s.upload != nil && !s.uploaded {
		_ = s.upload.Finish()
		fmt.Fprintln(os.Stderr)
		s.uploaded = true
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

// generateWFP fingerprints every file (standard WFP, via the shared
// scanner.GenerateWFP) and returns the combined WFP plus the number of files that
// failed to process.
func generateWFP(files []string, threads int, root string, bar *progressbar.ProgressBar) ([]byte, int) {
	wfp, errs := scanner.GenerateWFP(files, threads, root, func(done, total int) {
		if bar != nil {
			_ = bar.Set(done)
		}
	})
	for _, err := range errs {
		slog.Warn("file fingerprint failed", "err", err)
	}
	return wfp, len(errs)
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
	case config.FormatPlain, config.FormatSPDX, config.FormatCycloneDX:
		return nil
	default:
		return fmt.Errorf("invalid output format %q: must be plain, spdx, or cyclonedx", format)
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
	scanCmd.PersistentFlags().StringP("format", "f", config.DefaultFormat, "Result output format: plain, spdx, cyclonedx")
	scanCmd.PersistentFlags().String("settings", "", "Path to settings file (scanoss.json/settings.json)")
	scanCmd.PersistentFlags().Int("chunk-size", scanoss.DefaultScanChunkBytes, "WFP upload chunk size in bytes")
	scanCmd.PersistentFlags().Duration("poll-interval", scanoss.DefaultScanPollInterval, "How often to poll for scan status")
	scanCmd.PersistentFlags().Bool("ignore-cert-errors", false, "Ignore TLS certificate errors (insecure)")

	// Fingerprinting flags (apply to `scan <path>` only).
	scanCmd.Flags().IntP("threads", "t", config.DefaultThreads, "Number of parallel fingerprint workers")
	scanCmd.Flags().String("save-wfp", "", "Save WFP fingerprints to file before sending to API")
	scanCmd.Flags().Int64("max-size", 0, "Maximum file size in bytes to scan (0 = unlimited)")
	scanCmd.Flags().Bool("default-filters", true, "Apply the built-in default file filters")
	scanCmd.Flags().Bool("gitignore", true, "Honor .gitignore files when collecting files")
}

// runScan fingerprints a file or folder and scans the resulting WFP. A single
// file is scanned directly; a directory is collected (applying the filters) and
// every matching file is fingerprinted.
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

	targetPath := args[0]
	info, err := os.Stat(targetPath)
	if err != nil {
		return fmt.Errorf("error accessing path: %w", err)
	}

	threads, _ := cmd.Flags().GetInt("threads")
	saveWFPFile, _ := cmd.Flags().GetString("save-wfp")
	settingsFlag, _ := cmd.Flags().GetString("settings")
	maxSize, _ := cmd.Flags().GetInt64("max-size")
	applyDefaultFilters, _ := cmd.Flags().GetBool("default-filters")
	applyGitignore, _ := cmd.Flags().GetBool("gitignore")

	if threads < 1 {
		threads = config.DefaultThreads
	}

	// Settings drive file filtering. Of the BOM rules, only bom.remove is applied:
	// client-side, post-scan, via scanoss.WithBOM below (bom.include only protects
	// its purls from that removal). bom.include is not yet honored server-side, and
	// identify/ignore/replace are not applied.
	scanSettings, err := settings.Resolve(settingsFlag, targetPath)
	if err != nil {
		return fmt.Errorf("error loading settings: %w", err)
	}

	// Resolve the files to fingerprint: a directory is collected (and filtered),
	// a single file is scanned as-is.
	var files []string
	if info.IsDir() {
		collectResult, err := scanner.CollectFilesWithOptions(targetPath, filter.Options{
			MaxSize:   maxSize,
			Defaults:  applyDefaultFilters,
			GitIgnore: applyGitignore,
			Settings:  scanSettings.ScanFilter(),
		})
		if err != nil {
			return fmt.Errorf("error collecting files: %w", err)
		}
		files = collectResult.Files
		if collectResult.SkippedCount > 0 {
			fmt.Fprintf(os.Stderr, "Filtered %d files\n", collectResult.SkippedCount)
		}
	} else {
		abs, absErr := filepath.Abs(targetPath)
		if absErr != nil {
			abs = targetPath
		}
		files = []string{abs}
	}
	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "No valid files found to process\n")
		return nil
	}

	// Report result paths relative to the scanned root (the folder, or the file's
	// directory for a single-file scan), so the WFP carries relative labels.
	scanRoot := targetPath
	if !info.IsDir() {
		scanRoot = filepath.Dir(targetPath)
	}
	if abs, absErr := filepath.Abs(scanRoot); absErr == nil {
		scanRoot = abs
	}

	// Fingerprint all files and assemble the WFP.
	fpBar := progressbar.NewOptions(len(files),
		progressbar.OptionSetDescription("Fingerprinting"),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(40),
		progressbar.OptionOnCompletion(func() { fmt.Fprintln(os.Stderr) }),
	)
	wfp, procErrors := generateWFP(files, threads, scanRoot, fpBar)
	_ = fpBar.Finish()

	if len(wfp) == 0 {
		fmt.Fprintln(os.Stderr, "No fingerprints generated")
		printErrorSummary(procErrors)
		return nil
	}

	if saveWFPFile != "" {
		if err := os.WriteFile(saveWFPFile, wfp, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write WFP file: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "WFP fingerprints saved to: %s\n", saveWFPFile)
		}
	}

	return uploadAndWrite(cmd, wfp, scanSettings, procErrors, targetPath)
}

// runScanWFP uploads a pre-generated WFP file and scans it directly (no
// fingerprinting).
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

	return uploadAndWrite(cmd, wfp, scanSettings, 0, wfpPath)
}

// uploadAndWrite builds the SDK client, uploads the assembled WFP to the v3 batch
// scan API, applies any bom.remove rules from settings, and writes the result in the
// requested format (raw JSON, CycloneDX, or SPDX Lite). procErrors is the count of
// files that failed to fingerprint (0 for a WFP file). scanPath is the scan input path,
// used to name the SBOM project.
func uploadAndWrite(cmd *cobra.Command, wfp []byte, scanSettings *settings.Settings, procErrors int, scanPath string) error {
	apiURL, _ := cmd.Flags().GetString("api-url")
	apiKey, _ := cmd.Flags().GetString("api-key")
	outputFile, _ := cmd.Flags().GetString("output")
	chunkSize, _ := cmd.Flags().GetInt("chunk-size")
	pollInterval, _ := cmd.Flags().GetDuration("poll-interval")
	ignoreCertErrors, _ := cmd.Flags().GetBool("ignore-cert-errors")
	outputFormat, _ := cmd.Flags().GetString("format")

	if chunkSize < 1024 {
		chunkSize = scanoss.DefaultScanChunkBytes
	}

	writer, err := output.NewWriter(outputFile)
	if err != nil {
		return fmt.Errorf("error creating output writer: %w", err)
	}
	defer func() { _ = writer.Close() }()

	// Build the SDK client. The scan-id notify hook prints the recovery command.
	prog := &scanProgress{}
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
	client := scanoss.New(opts...)

	ctx, cancel := createCancellableContext()
	defer cancel()

	// The scan applies bom.remove itself when a BOM is configured (SDK-side).
	scanOpts := []scanoss.ScanOption{scanoss.WithChunkBytes(chunkSize), scanoss.WithPollInterval(pollInterval)}
	if scanSettings != nil && scanSettings.HasBOM() {
		scanOpts = append(scanOpts, scanoss.WithBOM(&scanSettings.BOM))
	}

	res, err := client.Scan.WFP(ctx, wfp, scanOpts...)
	if err != nil {
		return renderAPIError(fmt.Errorf("scan failed: %w", err))
	}
	if res.Result == nil {
		return fmt.Errorf("scan completed without a result")
	}

	out, err := renderResult(ctx, client, res.Result, outputFormat, scanPath)
	if err != nil {
		return err
	}
	if err := writer.Write(out); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing: %v\n", err)
	}
	printErrorSummary(procErrors)
	return nil
}

// renderResult turns a scan result into the requested output. "plain" is the raw v3
// result JSON; cyclonedx/spdx are built via the sbom module, with licenses (and, for
// cyclonedx, vulnerabilities) fetched from the decoration services (non-fatal on
// failure).
func renderResult(ctx context.Context, client *scanoss.Client, result *scanossapi.ScanResult, format, scanPath string) (string, error) {
	switch format {
	case config.FormatPlain:
		raw, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return "", fmt.Errorf("error encoding results: %w", err)
		}
		return string(raw), nil
	case config.FormatSPDX, config.FormatCycloneDX:
		// build the SBOM below
	default:
		return "", fmt.Errorf("unsupported output format: %q", format)
	}

	inv := scansource.FromScanResult(result)

	// Attach declared licenses from the decoration service, matched to each component
	// by PURL + version (the queried version, echoed back as the response requirement).
	if byKey := fetchLicenses(ctx, client, inv); len(byKey) > 0 {
		for i := range inv.Components {
			c := &inv.Components[i]
			c.Licenses = byKey[scansource.LicenseKey(c.Purl, c.Version)]
		}
	}

	if format == config.FormatCycloneDX {
		inv.Vulnerabilities = fetchVulnerabilities(ctx, client, inv)
	}

	projectName := filepath.Base(scanPath)
	if projectName == "." || projectName == "/" {
		projectName = "" // let the sbom module apply its default
	}
	out, err := sbom.Generate(inv, sbom.Format(format), sbom.WithProjectName(projectName))
	if err != nil {
		return "", fmt.Errorf("error generating %s output: %w", format, err)
	}
	return out, nil
}

// componentPurls returns the PURLs of the inventory's components.
func componentPurls(inv sbom.Inventory) []string {
	purls := make([]string, 0, len(inv.Components))
	for _, c := range inv.Components {
		purls = append(purls, c.Purl)
	}
	return purls
}

// fetchLicenses fetches declared licenses from the decoration service, keyed by PURL.
// Each component is queried at its matched version (as the requirement) so the service
// resolves the license for that version. Failure is non-fatal: it warns and returns nil
// so the SBOM is still produced.
func fetchLicenses(ctx context.Context, client *scanoss.Client, inv sbom.Inventory) map[string][]sbom.License {
	if len(inv.Components) == 0 {
		return nil
	}
	comps := make([]scanoss.Component, 0, len(inv.Components))
	for _, c := range inv.Components {
		comps = append(comps, scanoss.Component{Purl: c.Purl, Requirement: c.Version})
	}
	resp, err := client.Licenses.Components(ctx, comps)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not fetch licenses: %v\n", err)
		return nil
	}
	return scansource.LicensesFrom(resp)
}

// fetchVulnerabilities decorates the inventory's components with known vulnerabilities
// via the decoration service. Failure is non-fatal: it warns and returns nil so the
// SBOM is still produced.
func fetchVulnerabilities(ctx context.Context, client *scanoss.Client, inv sbom.Inventory) []sbom.Vulnerability {
	purls := componentPurls(inv)
	if len(purls) == 0 {
		return nil
	}
	resp, err := client.Vulnerabilities.Components(ctx, scanoss.Components(purls...))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not fetch vulnerabilities: %v\n", err)
		return nil
	}
	return scansource.VulnerabilitiesFrom(resp)
}
