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

	"github.com/scanoss/scanoss.go/internal/config"
	"github.com/scanoss/scanoss.go/pkg/output"
	"github.com/scanoss/scanoss.go/pkg/scanoss"
	"github.com/spf13/cobra"
)

var resultsCmd = &cobra.Command{
	Use:   "results <scan-id>",
	Short: "Retrieve results from a previous batch scan using its scan id",
	Long: `The results command resumes a previous v3 batch scan by its scan id and
polls the API until the results are ready.

Examples:
  # Get results from a previous scan
  scanoss results 05b7ede7702c3a7c6ccec7f23252ce47

  # Get results and save to file
  scanoss results 05b7ede7702c3a7c6ccec7f23252ce47 --output results.json

  # Get results with custom API URL
  scanoss results 05b7ede7702c3a7c6ccec7f23252ce47 --api-url https://api.scanoss.com`,
	Args: cobra.MaximumNArgs(1),
	RunE: runResults,
}

func init() {
	rootCmd.AddCommand(resultsCmd)

	resultsCmd.Flags().String("api-url", config.DefaultAPIURL, "SCANOSS API URL")
	resultsCmd.Flags().String("api-key", "", "API authentication token")
	resultsCmd.Flags().StringP("output", "o", "", "Output file (empty = stdout)")
	resultsCmd.Flags().Duration("poll-interval", scanoss.DefaultScanPollInterval, "How often to poll for scan status")
}

func runResults(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	if err := checkAuth(cmd); err != nil {
		return err
	}

	scanID := args[0]

	apiURL, _ := cmd.Flags().GetString("api-url")
	apiKey, _ := cmd.Flags().GetString("api-key")
	outputFile, _ := cmd.Flags().GetString("output")
	pollInterval, _ := cmd.Flags().GetDuration("poll-interval")

	writer, err := output.NewWriter(outputFile)
	if err != nil {
		return fmt.Errorf("error creating output writer: %w", err)
	}
	defer func() { _ = writer.Close() }()

	prog := &scanProgress{}
	client := scanoss.New(
		scanoss.WithAPIURL(apiURL),
		scanoss.WithAPIKey(apiKey),
		scanoss.WithProgress(prog.fn),
	)

	infof("Retrieving results for scan %s", scanID)

	ctx, cancel := createCancellableContext()
	defer cancel()

	res, err := client.Scan.Wait(ctx, scanID, scanoss.WithPollInterval(pollInterval))
	if err != nil {
		return renderAPIError(fmt.Errorf("failed to retrieve results: %w", err))
	}
	if res.Result == nil {
		return fmt.Errorf("scan completed without a result")
	}

	out, err := json.MarshalIndent(res.Result, "", "  ")
	if err != nil {
		return fmt.Errorf("error encoding results: %w", err)
	}
	if err := writer.Write(string(out)); err != nil {
		return fmt.Errorf("error writing results: %w", err)
	}
	return nil
}
