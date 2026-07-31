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
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"github.com/scanoss/scanoss.go/internal/cliconfig"
	"github.com/scanoss/scanoss.go/pkg/dependencies"
	"github.com/scanoss/scanoss.go/pkg/dependencies/parsers"
	"github.com/scanoss/scanoss.go/pkg/filter"
	"github.com/scanoss/scanoss.go/pkg/scanoss"
	"github.com/scanoss/scanoss.go/pkg/settings"
)

var dependenciesCmd = &cobra.Command{
	Use:   "dependencies [path]",
	Short: "Extract local dependencies or query SCANOSS API for component dependencies",
	Long: `The dependencies command has three modes:

1. LOCAL MODE (--extract-local):
   Recursively scans a directory to find dependency manifest files and extracts
   dependency information as standardized Package URLs (PURLs).

   Supported ecosystems:
     - Node.js:      package.json, package-lock.json, yarn.lock
     - Python:       requirements.txt, pyproject.toml
     - Java (Maven): pom.xml
     - Java (Gradle): build.gradle
     - Go:           go.mod, go.sum
     - Ruby:         Gemfile, Gemfile.lock
     - .NET:         *.csproj, packages.config

2. API MODE (--purl):
   Queries the SCANOSS API for component dependencies (direct or transitive).

3. SCAN MODE (default):
   Extracts local dependencies from the provided path and queries the SCANOSS API
   for dependency information (vulnerabilities, licenses, etc.).

Examples:
  # Extract local dependencies and display on console
  scanoss-cli dependencies ./my-project --extract-local

  # Extract local dependencies and save to file
  scanoss-cli dependencies ./my-project --extract-local --output deps.json

  # Scan project dependencies and query API for direct dependencies
  scanoss-cli dependencies ./my-project

  # Scan project dependencies and query API for transitive dependencies
  scanoss-cli dependencies ./my-project --transient

  # Query direct dependencies from API for specific component
  scanoss-cli dependencies --purl 'pkg:github/scanoss/engine' --requirement '5.4.7'

  # Query transitive dependencies from API for specific component
  scanoss-cli dependencies --purl 'pkg:github/scanoss/engine' --requirement '5.4.7' --transient

  # With custom depth and limit for transitive
  scanoss-cli dependencies --purl 'pkg:github/scanoss/engine' --requirement '5.4.7' --transient --depth 5 --limit 20`,
	RunE: runDependencies,
}

func init() {
	rootCmd.AddCommand(dependenciesCmd)

	// Local extraction mode
	dependenciesCmd.Flags().Bool("extract-local", false, "Extract local dependencies from manifest files")
	dependenciesCmd.Flags().String("settings", "", "Path to settings file (scanoss.json/settings.json)")

	// API mode flags
	dependenciesCmd.Flags().String("purl", "", "Package URL (purl) of the component (required for API mode)")
	dependenciesCmd.Flags().String("requirement", "", "Version requirement (required for API mode)")

	// Mode flag
	dependenciesCmd.Flags().Bool("transient", false, "Query transitive dependencies (API mode)")

	// Transitive-specific flags
	dependenciesCmd.Flags().Int("depth", 10, "Depth for transitive dependencies (only with --transient)")
	dependenciesCmd.Flags().Int("limit", 10, "Limit for transitive dependencies (only with --transient)")

	// API and output configuration
	addAPIFlags(dependenciesCmd)
}

func runDependencies(cmd *cobra.Command, args []string) error {
	extractLocal, _ := cmd.Flags().GetBool("extract-local")
	settingsFlag, _ := cmd.Flags().GetString("settings")
	outputFile, _ := cmd.Flags().GetString("output")
	purl, _ := cmd.Flags().GetString("purl")
	transient, _ := cmd.Flags().GetBool("transient")
	depth, _ := cmd.Flags().GetInt("depth")
	limit, _ := cmd.Flags().GetInt("limit")
	api, err := cliconfig.ResolveAPI(cmd.Flags())
	if err != nil {
		return err
	}
	// Kept as locals: the query helpers below take the endpoint and key as strings.
	apiURL, apiKey := api.URL, api.Key
	httpClient, err := newHTTPClient(cmd)
	if err != nil {
		return err
	}

	if !extractLocal && purl == "" && len(args) == 0 {
		return usageError(cmd, "a path, --purl or --extract-local is required")
	}

	// Mode 1: Local extraction only
	if extractLocal {
		if len(args) == 0 {
			return fmt.Errorf("path argument is required with --extract-local")
		}
		return runLocalExtraction(args[0], outputFile, settingsFlag)
	}

	// Mode 2: API query with specific purl. --requirement is optional (the API
	// treats it as optional, and the purl itself may carry the version).
	if purl != "" {
		requirement, _ := cmd.Flags().GetString("requirement")

		var response string
		var err error

		if transient {
			// Transitive dependencies endpoint
			response, err = queryTransitiveDependencies(httpClient, apiURL, apiKey, purl, requirement, depth, limit)
		} else {
			// Direct dependencies endpoint
			response, err = queryDirectDependencies(httpClient, apiURL, apiKey, purl, requirement)
		}

		if err != nil {
			return fmt.Errorf("error querying dependencies: %w", err)
		}

		return writeOutput(response, outputFile)
	}

	// Mode 3: Scan mode - extract local dependencies and query API
	if len(args) == 0 {
		return fmt.Errorf("path argument is required (use --purl for API-only mode or --extract-local for local-only mode)")
	}

	return runScanMode(httpClient, args[0], outputFile, apiURL, apiKey, settingsFlag, transient, depth, limit)
}

// runLocalExtraction extracts dependencies from local manifest files
func runLocalExtraction(targetPath, outputFile, settingsFlag string) error {
	// Check if path exists
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", targetPath)
	}

	// Create dependency parser
	parser := dependencies.NewDependencyParser()

	infof("Scanning for dependency files in %s", targetPath)

	// Collect the candidate files
	allFiles, skipped, err := collectDependencyFiles(targetPath, settingsFlag)
	if err != nil {
		return fmt.Errorf("failed to collect files: %w", err)
	}

	infof("Total files found: %d", len(allFiles))
	if skipped > 0 {
		infof("Filtered %d files", skipped)
	}

	// Filter only dependency files
	depFiles := parser.FilterFiles(allFiles)

	if len(depFiles) == 0 {
		warnf("No dependency files found")

		// Return empty result
		emptyResult := &parsers.LocalDependencies{
			Files: []parsers.LocalDependency{},
		}

		return outputJSON(emptyResult, outputFile)
	}

	infof("Dependency files found: %d", len(depFiles))
	fmt.Fprintf(os.Stderr, "\n")

	// Create progress bar
	prog := newProgress()
	bar := addBar(prog, len(depFiles), "Parsing dependencies")

	// Parse all dependency files
	result := &parsers.LocalDependencies{
		Files: []parsers.LocalDependency{},
	}

	for _, filePath := range depFiles {
		dep, err := parser.ParseFile(filePath)
		if err != nil {
			warnf("failed to parse %s: %v", filePath, err)
			bar.Increment()
			continue
		}

		// Only add if there are dependencies
		if len(dep.Purls) > 0 {
			result.Files = append(result.Files, *dep)
		}

		bar.Increment()
	}

	bar.SetCurrent(int64(len(depFiles)))
	prog.Wait()

	// Count total dependencies
	totalDeps := 0
	for _, file := range result.Files {
		totalDeps += len(file.Purls)
	}

	okf("Successfully parsed %d files", len(result.Files))
	infof("Total dependencies found: %d", totalDeps)
	fmt.Fprintf(os.Stderr, "\n")

	return outputJSON(result, outputFile)
}

// runScanMode extracts local dependencies and queries the SCANOSS API
func runScanMode(httpClient *http.Client, targetPath, outputFile, apiURL, apiKey, settingsFlag string, transient bool, depth, limit int) error {
	// Check if path exists
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", targetPath)
	}

	// Create dependency parser
	parser := dependencies.NewDependencyParser()

	infof("Scanning for dependency files in %s", targetPath)

	// Collect the candidate files
	allFiles, skipped, err := collectDependencyFiles(targetPath, settingsFlag)
	if err != nil {
		return fmt.Errorf("failed to collect files: %w", err)
	}

	infof("Total files found: %d", len(allFiles))
	if skipped > 0 {
		infof("Filtered %d files", skipped)
	}

	// Filter only dependency files
	depFiles := parser.FilterFiles(allFiles)

	if len(depFiles) == 0 {
		return fmt.Errorf("no dependency files found in %s", targetPath)
	}

	infof("Dependency files found: %d", len(depFiles))
	fmt.Fprintf(os.Stderr, "\n")

	// Create progress bar
	prog := newProgress()
	bar := addBar(prog, len(depFiles), "Parsing dependencies")

	// Parse all dependency files
	localDeps := &parsers.LocalDependencies{
		Files: []parsers.LocalDependency{},
	}

	for _, filePath := range depFiles {
		dep, err := parser.ParseFile(filePath)
		if err != nil {
			warnf("failed to parse %s: %v", filePath, err)
			bar.Increment()
			continue
		}

		// Only add if there are dependencies
		if len(dep.Purls) > 0 {
			localDeps.Files = append(localDeps.Files, *dep)
		}

		bar.Increment()
	}

	bar.SetCurrent(int64(len(depFiles)))
	prog.Wait()

	// Count total dependencies
	totalDeps := 0
	for _, file := range localDeps.Files {
		totalDeps += len(file.Purls)
	}

	okf("Successfully parsed %d files", len(localDeps.Files))
	infof("Total dependencies found: %d", totalDeps)
	fmt.Fprintf(os.Stderr, "\n")

	if totalDeps == 0 {
		return fmt.Errorf("no dependencies found in any files")
	}

	// Query SCANOSS API with extracted dependencies
	infof("Querying SCANOSS API...")

	var response string
	if transient {
		response, err = queryTransitiveDependenciesWithFiles(httpClient, apiURL, apiKey, localDeps, depth, limit)
	} else {
		response, err = queryDirectDependenciesWithFiles(httpClient, apiURL, apiKey, localDeps)
	}

	if err != nil {
		return fmt.Errorf("error querying SCANOSS API: %w", err)
	}

	okf("API query successful")

	return writeOutput(response, outputFile)
}

// collectDependencyFiles collects the files a dependency scan may parse, using
// the shared filter with the dependency profile: the same rules the rest of the
// CLI applies, and the same ones `scan --include deps` uses for this stage.
//
// It replaces a hand-written walk that carried its own directory list and read
// neither scanoss.json nor the shared defaults. The manifests survive the
// default extension list (.json, .mod, .xml, …) through
// PreserveDependencyManifests, so what the parser receives is unchanged.
func collectDependencyFiles(root, settingsFlag string) ([]string, int, error) {
	depSettings, err := settings.Resolve(settingsFlag, root)
	if err != nil {
		return nil, 0, fmt.Errorf("error loading settings: %w", err)
	}
	opts := filter.DependencyOptions()
	opts.Settings = depSettings.DependencyFilter()

	res, err := filter.Collect(root, opts)
	if err != nil {
		return nil, 0, err
	}
	return res.Files, res.SkippedCount, nil
}

// outputJSON marshals data to JSON and writes to file or stdout
func outputJSON(data interface{}, outputFile string) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to generate JSON: %w", err)
	}

	if outputFile != "" {
		err := os.WriteFile(outputFile, jsonData, 0644)
		if err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		okf("Results saved to %s", outputFile)
	} else {
		fmt.Println(string(jsonData))
	}

	return nil
}

// depClient builds an SDK client on the transport the command already configured
// from --proxy, --ca-cert and --ignore-cert-errors.
func depClient(httpClient *http.Client, apiURL, apiKey string) (*scanoss.Client, error) {
	return scanoss.New(scanoss.Config{
		APIURL:     apiURL,
		APIKey:     apiKey,
		HTTPClient: httpClient,
	})
}

// componentsFromLocalDeps flattens the per-file extracted PURLs into a single
// component list for the v3 batch / transitive endpoints.
func componentsFromLocalDeps(localDeps *parsers.LocalDependencies) []scanoss.Component {
	var comps []scanoss.Component
	for _, file := range localDeps.Files {
		for _, p := range file.Purls {
			comps = append(comps, scanoss.Component{Purl: p.Purl, Requirement: p.Requirement})
		}
	}
	return comps
}

// marshalDepResponse pretty-prints a typed SDK response as JSON for CLI output.
func marshalDepResponse(v interface{}) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error encoding response: %w", err)
	}
	return string(b), nil
}

// queryDirectDependencies resolves declared dependencies for a single purl via the
// v3 dependencies endpoint (SDK-typed response).
func queryDirectDependencies(httpClient *http.Client, apiURL, apiKey, purl, requirement string) (string, error) {
	client, err := depClient(httpClient, apiURL, apiKey)
	if err != nil {
		return "", err
	}
	resp, err := client.Dependencies.Dependency(context.Background(), scanoss.Component{Purl: purl, Requirement: requirement})
	if err != nil {
		return "", err
	}
	return marshalDepResponse(resp)
}

// queryTransitiveDependencies walks the transitive dependency tree for a single purl.
func queryTransitiveDependencies(httpClient *http.Client, apiURL, apiKey, purl, requirement string, depth, limit int) (string, error) {
	client, err := depClient(httpClient, apiURL, apiKey)
	if err != nil {
		return "", err
	}
	resp, err := client.Dependencies.Transitive(context.Background(), []scanoss.Component{{Purl: purl, Requirement: requirement}}, depth, limit)
	if err != nil {
		return "", err
	}
	return marshalDepResponse(resp)
}

// queryDirectDependenciesWithFiles resolves declared dependencies for all PURLs
// extracted from local manifests (v3 batch endpoint).
func queryDirectDependenciesWithFiles(httpClient *http.Client, apiURL, apiKey string, localDeps *parsers.LocalDependencies) (string, error) {
	client, err := depClient(httpClient, apiURL, apiKey)
	if err != nil {
		return "", err
	}
	resp, err := client.Dependencies.Dependencies(context.Background(), componentsFromLocalDeps(localDeps))
	if err != nil {
		return "", err
	}
	return marshalDepResponse(resp)
}

// queryTransitiveDependenciesWithFiles walks the transitive tree for all PURLs
// extracted from local manifests.
func queryTransitiveDependenciesWithFiles(httpClient *http.Client, apiURL, apiKey string, localDeps *parsers.LocalDependencies, depth, limit int) (string, error) {
	client, err := depClient(httpClient, apiURL, apiKey)
	if err != nil {
		return "", err
	}
	resp, err := client.Dependencies.Transitive(context.Background(), componentsFromLocalDeps(localDeps), depth, limit)
	if err != nil {
		return "", err
	}
	return marshalDepResponse(resp)
}
