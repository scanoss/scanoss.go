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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/scanoss/scanoss.go/internal/config"
	"github.com/spf13/cobra"
)

var attributionsCmd = &cobra.Command{
	Use:   "attributions [sbom-file]",
	Short: "Generate attribution text from an SBOM file or PURL using SCANOSS API",
	Long: `The attributions command sends an SBOM file (or creates one from a PURL)
to the SCANOSS API and retrieves attribution text in plain text format.

You can either provide an existing SBOM file or use the --purl flag to
generate attributions for a specific package.

Examples:
  # Generate attributions from SBOM file to console
  scanoss attributions sbom.json

  # Generate attributions and save to file
  scanoss attributions sbom.json --output attributions.txt

  # Generate attributions from a PURL
  scanoss attributions --purl "pkg:github/scanoss/engine@v5.4.19"

  # Generate attributions from PURL and save to file
  scanoss attributions --purl "pkg:github/scanoss/engine@v5.4.19" --output attributions.txt

  # Use custom API URL
  scanoss attributions sbom.json --api-url https://api.scanoss.com

  # With API key authentication
  scanoss attributions --purl "pkg:github/scanoss/engine@v5.4.19" --api-key YOUR_API_KEY

  # Complete example with file
  scanoss attributions MySBOM.json --api-url https://api.scanoss.com --api-key 123456 --output myRequestedAttributions.txt

  # Complete example with PURL
  scanoss attributions --purl "pkg:github/scanoss/engine@v5.4.19" --api-url https://api.scanoss.com --api-key 123456 --output attributions.txt`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAttributions,
}

func init() {
	rootCmd.AddCommand(attributionsCmd)

	// Input mode
	attributionsCmd.Flags().String("purl", "", "Package URL to query for attributions (alternative to providing a file)")

	// API configuration
	attributionsCmd.Flags().String("api-url", config.DefaultAPIURL, "SCANOSS API base URL")
	attributionsCmd.Flags().String("api-key", "", "API key for authentication")
	attributionsCmd.Flags().Bool("ignore-cert-errors", false, "Ignore TLS certificate errors (insecure)")

	// Output configuration
	attributionsCmd.Flags().StringP("output", "o", "", "Output file (default: stdout)")
}

func runAttributions(cmd *cobra.Command, args []string) error {
	purl, _ := cmd.Flags().GetString("purl")

	// No input: show usage. Both an SBOM file and --purl: error.
	if len(args) == 0 && purl == "" {
		return cmd.Help()
	}
	if len(args) > 0 && purl != "" {
		return fmt.Errorf("cannot specify both SBOM file and --purl flag")
	}

	if err := checkAuth(cmd); err != nil {
		return err
	}

	apiURL, _ := cmd.Flags().GetString("api-url")
	apiKey, _ := cmd.Flags().GetString("api-key")
	outputFile, _ := cmd.Flags().GetString("output")
	ignoreCertErrors, _ := cmd.Flags().GetBool("ignore-cert-errors")

	if ignoreCertErrors {
		slog.Warn("ignoring TLS certificate errors (insecure)")
	}

	var sbomFilePath string
	var tempFile bool

	// Case 1: SBOM file provided
	if len(args) > 0 {
		sbomFilePath = args[0]
		// Verify that SBOM file exists
		if _, err := os.Stat(sbomFilePath); os.IsNotExist(err) {
			return fmt.Errorf("SBOM file does not exist: %s", sbomFilePath)
		}
	} else {
		// Case 2: PURL provided - create temporary SBOM file
		tempFilePath, err := createTempSBOMFromPURL(purl)
		if err != nil {
			return fmt.Errorf("error creating temporary SBOM: %w", err)
		}
		sbomFilePath = tempFilePath
		tempFile = true
		// Ensure cleanup of temp file
		defer func() { _ = os.Remove(tempFilePath) }()
	}

	// Send SBOM file to API
	attributionText, err := sendSBOMForAttributions(apiURL, apiKey, sbomFilePath, ignoreCertErrors)
	if err != nil {
		if tempFile {
			// Clean up temp file before returning error
			_ = os.Remove(sbomFilePath)
		}
		return fmt.Errorf("error getting attributions: %w", err)
	}

	// Write output
	if err := writeAttributionsOutput(attributionText, outputFile); err != nil {
		return fmt.Errorf("error writing output: %w", err)
	}

	return nil
}

// createTempSBOMFromPURL creates a temporary JSON file with the PURL
func createTempSBOMFromPURL(purl string) (string, error) {
	// Create temporary file
	tempFile, err := os.CreateTemp("", "scanoss-sbom-*.json")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() { _ = tempFile.Close() }()

	// Create JSON content
	sbomContent := map[string]string{
		"purl": purl,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(sbomContent)
	if err != nil {
		_ = os.Remove(tempFile.Name())
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Write to temp file
	if _, err := tempFile.Write(jsonData); err != nil {
		_ = os.Remove(tempFile.Name())
		return "", fmt.Errorf("failed to write temp file: %w", err)
	}

	return tempFile.Name(), nil
}

// sendSBOMForAttributions sends the SBOM file to the API and returns attribution text
func sendSBOMForAttributions(apiURL, apiKey, sbomFilePath string, insecure bool) (string, error) {
	// Clean up URL to avoid double slashes
	apiURL = strings.TrimSuffix(apiURL, "/")
	endpoint := apiURL + "/sbom/attribution"

	// Open SBOM file
	file, err := os.Open(sbomFilePath)
	if err != nil {
		return "", fmt.Errorf("error opening SBOM file: %w", err)
	}
	defer func() { _ = file.Close() }()

	// Create multipart form body
	bodyBuf := &bytes.Buffer{}
	bodyWriter := multipart.NewWriter(bodyBuf)

	// Add file to form
	fileWriter, err := bodyWriter.CreateFormFile("file", filepath.Base(sbomFilePath))
	if err != nil {
		return "", fmt.Errorf("error creating form file: %w", err)
	}

	_, err = io.Copy(fileWriter, file)
	if err != nil {
		return "", fmt.Errorf("error copying file content: %w", err)
	}

	contentType := bodyWriter.FormDataContentType()
	_ = bodyWriter.Close()

	// Create HTTP request
	req, err := http.NewRequest("POST", endpoint, bodyBuf)
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", contentType)
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("x-Session", apiKey)
	}

	// Make request
	client := newHTTPClient(insecure)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(responseBody))
	}

	return string(responseBody), nil
}

// writeAttributionsOutput writes the attribution text to stdout or a file.
// JSON responses are pretty-printed (2-space indent); non-JSON is written as-is.
func writeAttributionsOutput(content, outputFile string) error {
	var buf bytes.Buffer
	if json.Indent(&buf, []byte(content), "", "  ") == nil {
		content = buf.String()
	}

	if outputFile == "" {
		// Write to stdout
		fmt.Print(content)
		return nil
	}

	// Write to file
	if err := os.WriteFile(outputFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("error writing to file: %w", err)
	}

	okf("Attributions saved to %s", outputFile)
	return nil
}
