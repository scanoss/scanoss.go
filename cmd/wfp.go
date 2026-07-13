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
	"os"
	"path/filepath"
	"time"

	"github.com/scanoss/scanoss.go/internal/config"
	"github.com/scanoss/scanoss.go/pkg/output"
	"github.com/scanoss/scanoss.go/pkg/scanner"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

var wfpCmd = &cobra.Command{
	Use:   "wfp <path>",
	Short: "Generate WFP fingerprints from a file or folder",
	Long: `The wfp command scans a file or recursively scans a folder, generates WFP
(Winnowing Fingerprint) fingerprints for each valid file and displays
the results on standard output.

This command is useful for:
  - Inspecting generated fingerprints
  - Debugging
  - Generating WFP files for offline processing`,
	Args: cobra.ExactArgs(1),
	RunE: runWFP,
}

func init() {
	rootCmd.AddCommand(wfpCmd)

	// Optional flags
	wfpCmd.Flags().IntP("threads", "t", config.DefaultThreads, "Number of parallel threads")
	wfpCmd.Flags().StringP("output", "o", "", "Output file (empty = stdout)")
}

func runWFP(cmd *cobra.Command, args []string) error {
	targetPath := args[0]

	// Verify that the path exists (can be a file or folder)
	info, err := os.Stat(targetPath)
	if err != nil {
		return fmt.Errorf("error accessing path: %w", err)
	}

	// Get configuration from flags
	threads, _ := cmd.Flags().GetInt("threads")
	outputFile, _ := cmd.Flags().GetString("output")

	// Validate configuration
	if threads < 1 {
		threads = config.DefaultThreads
	}

	// Report WFP paths relative to the scanned root (a folder, or the file's
	// directory for a single-file target), matching the `scan` command.
	scanRoot := targetPath
	if !info.IsDir() {
		scanRoot = filepath.Dir(targetPath)
	}
	if abs, absErr := filepath.Abs(scanRoot); absErr == nil {
		scanRoot = abs
	}

	// Collect files
	files, err := scanner.CollectFiles(targetPath)
	if err != nil {
		return fmt.Errorf("error collecting files: %w", err)
	}

	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "No valid files found to process\n")
		return nil
	}

	// Create output writer
	writer, err := output.NewWriter(outputFile)
	if err != nil {
		return fmt.Errorf("error creating output writer: %w", err)
	}
	defer func() { _ = writer.Close() }()

	// Create progress bar
	bar := progressbar.NewOptions(len(files),
		progressbar.OptionSetDescription("Processing files"),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSetWidth(40),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionOnCompletion(func() {
			fmt.Fprintf(os.Stderr, "\n")
		}),
	)

	// Generate the WFP through the shared path (same as `scan`), with paths
	// relative to the scan root.
	wfp, errs := scanner.GenerateWFP(files, threads, scanRoot, func(done, total int) {
		_ = bar.Set(done)
	})
	_ = bar.Finish()

	if err := writer.Write(string(wfp)); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing: %v\n", err)
	}
	if len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "Completed with %d errors\n", len(errs))
	}

	return nil
}
