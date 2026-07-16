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
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/scanoss/scanoss.go/pkg/dependencies"
	"github.com/scanoss/scanoss.go/pkg/dependencies/parsers"
	"github.com/scanoss/scanoss.go/pkg/scanoss"
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
  scanoss dependencies ./my-project --extract-local

  # Extract local dependencies and save to file
  scanoss dependencies ./my-project --extract-local --output deps.json

  # Scan project dependencies and query API for direct dependencies
  scanoss dependencies ./my-project

  # Scan project dependencies and query API for transitive dependencies
  scanoss dependencies ./my-project --transient

  # Query direct dependencies from API for specific component
  scanoss dependencies --purl 'pkg:github/scanoss/engine' --requirement '5.4.7'

  # Query transitive dependencies from API for specific component
  scanoss dependencies --purl 'pkg:github/scanoss/engine' --requirement '5.4.7' --transient

  # With custom depth and limit for transitive
  scanoss dependencies --purl 'pkg:github/scanoss/engine' --requirement '5.4.7' --transient --depth 5 --limit 20`,
	RunE: runDependencies,
}

func init() {
	rootCmd.AddCommand(dependenciesCmd)

	// Local extraction mode
	dependenciesCmd.Flags().Bool("extract-local", false, "Extract local dependencies from manifest files")

	// API mode flags
	dependenciesCmd.Flags().String("purl", "", "Package URL (purl) of the component (required for API mode)")
	dependenciesCmd.Flags().String("requirement", "", "Version requirement (required for API mode)")

	// Mode flag
	dependenciesCmd.Flags().Bool("transient", false, "Query transitive dependencies (API mode)")

	// Transitive-specific flags
	dependenciesCmd.Flags().Int("depth", 10, "Depth for transitive dependencies (only with --transient)")
	dependenciesCmd.Flags().Int("limit", 10, "Limit for transitive dependencies (only with --transient)")

	// API configuration
	dependenciesCmd.Flags().String("api-url", "https://api.scanoss.com", "SCANOSS API base URL")
	dependenciesCmd.Flags().String("api-key", "", "API key for authentication")
	dependenciesCmd.Flags().Bool("ignore-cert-errors", false, "Ignore TLS certificate errors (insecure)")

	// Output configuration
	dependenciesCmd.Flags().StringP("output", "o", "", "Output file (default: stdout)")
}

func runDependencies(cmd *cobra.Command, args []string) error {
	extractLocal, _ := cmd.Flags().GetBool("extract-local")
	outputFile, _ := cmd.Flags().GetString("output")
	purl, _ := cmd.Flags().GetString("purl")
	transient, _ := cmd.Flags().GetBool("transient")
	depth, _ := cmd.Flags().GetInt("depth")
	limit, _ := cmd.Flags().GetInt("limit")
	apiURL, _ := cmd.Flags().GetString("api-url")
	apiKey, _ := cmd.Flags().GetString("api-key")
	ignoreCertErrors, _ := cmd.Flags().GetBool("ignore-cert-errors")

	if ignoreCertErrors {
		slog.Warn("ignoring TLS certificate errors (insecure)")
	}

	// No input (no path, no --purl, not --extract-local): show usage.
	if !extractLocal && purl == "" && len(args) == 0 {
		return cmd.Help()
	}

	// Mode 1: Local extraction only
	if extractLocal {
		if len(args) == 0 {
			return fmt.Errorf("path argument is required with --extract-local")
		}
		return runLocalExtraction(args[0], outputFile)
	}

	// Mode 2: API query with specific purl. --requirement is optional (the API
	// treats it as optional, and the purl itself may carry the version).
	if purl != "" {
		requirement, _ := cmd.Flags().GetString("requirement")

		var response string
		var err error

		if transient {
			// Transitive dependencies endpoint
			response, err = queryTransitiveDependencies(apiURL, apiKey, purl, requirement, depth, limit, ignoreCertErrors)
		} else {
			// Direct dependencies endpoint
			response, err = queryDirectDependencies(apiURL, apiKey, purl, requirement, ignoreCertErrors)
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

	return runScanMode(args[0], outputFile, apiURL, apiKey, transient, depth, limit, ignoreCertErrors)
}

// runLocalExtraction extracts dependencies from local manifest files
func runLocalExtraction(targetPath, outputFile string) error {
	// Check if path exists
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", targetPath)
	}

	// Create dependency parser
	parser := dependencies.NewDependencyParser()

	infof("Scanning for dependency files in %s", targetPath)

	// Collect all files recursively
	allFiles, err := collectFilesRecursively(targetPath)
	if err != nil {
		return fmt.Errorf("failed to collect files: %w", err)
	}

	infof("Total files found: %d", len(allFiles))

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
func runScanMode(targetPath, outputFile, apiURL, apiKey string, transient bool, depth, limit int, insecure bool) error {
	// Check if path exists
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", targetPath)
	}

	// Create dependency parser
	parser := dependencies.NewDependencyParser()

	infof("Scanning for dependency files in %s", targetPath)

	// Collect all files recursively
	allFiles, err := collectFilesRecursively(targetPath)
	if err != nil {
		return fmt.Errorf("failed to collect files: %w", err)
	}

	infof("Total files found: %d", len(allFiles))

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
		response, err = queryTransitiveDependenciesWithFiles(apiURL, apiKey, localDeps, depth, limit, insecure)
	} else {
		response, err = queryDirectDependenciesWithFiles(apiURL, apiKey, localDeps, insecure)
	}

	if err != nil {
		return fmt.Errorf("error querying SCANOSS API: %w", err)
	}

	okf("API query successful")

	return writeOutput(response, outputFile)
}

// collectFilesRecursively walks through directory and collects all file paths
func collectFilesRecursively(root string) ([]string, error) {
	var files []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Don't skip the root directory itself
		if path != root {
			// Skip hidden directories and files
			if info.IsDir() && len(info.Name()) > 0 && info.Name()[0] == '.' {
				return filepath.SkipDir
			}

			// Skip hidden files
			if !info.IsDir() && len(info.Name()) > 0 && info.Name()[0] == '.' {
				return nil
			}
		}

		// Skip directories like node_modules, vendor, etc.
		if info.IsDir() {
			switch info.Name() {
			case "node_modules", "vendor", ".git", ".svn", "dist", "build", "target", "__pycache__":
				return filepath.SkipDir
			}
		}

		if !info.IsDir() {
			files = append(files, path)
		}

		return nil
	})

	return files, err
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

// depClient builds an SDK client from the CLI's api-url / api-key / insecure flags.
func depClient(apiURL, apiKey string, insecure bool) *scanoss.Client {
	return scanoss.New(
		scanoss.WithAPIURL(apiURL),
		scanoss.WithAPIKey(apiKey),
		scanoss.WithInsecureTLS(insecure),
	)
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
func queryDirectDependencies(apiURL, apiKey, purl, requirement string, insecure bool) (string, error) {
	resp, err := depClient(apiURL, apiKey, insecure).
		Dependencies.Dependency(context.Background(), scanoss.Component{Purl: purl, Requirement: requirement})
	if err != nil {
		return "", err
	}
	return marshalDepResponse(resp)
}

// queryTransitiveDependencies walks the transitive dependency tree for a single purl.
func queryTransitiveDependencies(apiURL, apiKey, purl, requirement string, depth, limit int, insecure bool) (string, error) {
	resp, err := depClient(apiURL, apiKey, insecure).
		Dependencies.Transitive(context.Background(), []scanoss.Component{{Purl: purl, Requirement: requirement}}, depth, limit)
	if err != nil {
		return "", err
	}
	return marshalDepResponse(resp)
}

// queryDirectDependenciesWithFiles resolves declared dependencies for all PURLs
// extracted from local manifests (v3 batch endpoint).
func queryDirectDependenciesWithFiles(apiURL, apiKey string, localDeps *parsers.LocalDependencies, insecure bool) (string, error) {
	resp, err := depClient(apiURL, apiKey, insecure).
		Dependencies.Dependencies(context.Background(), componentsFromLocalDeps(localDeps))
	if err != nil {
		return "", err
	}
	return marshalDepResponse(resp)
}

// queryTransitiveDependenciesWithFiles walks the transitive tree for all PURLs
// extracted from local manifests.
func queryTransitiveDependenciesWithFiles(apiURL, apiKey string, localDeps *parsers.LocalDependencies, depth, limit int, insecure bool) (string, error) {
	resp, err := depClient(apiURL, apiKey, insecure).
		Dependencies.Transitive(context.Background(), componentsFromLocalDeps(localDeps), depth, limit)
	if err != nil {
		return "", err
	}
	return marshalDepResponse(resp)
}
