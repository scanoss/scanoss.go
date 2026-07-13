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

package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Writer handles writing results to console or file
type Writer struct {
	file   *os.File
	writer io.Writer
}

// NewWriter creates a new writer
// If outputPath is empty, writes to stdout
func NewWriter(outputPath string) (*Writer, error) {
	w := &Writer{}

	if outputPath == "" {
		w.writer = os.Stdout
	} else {
		file, err := os.Create(outputPath)
		if err != nil {
			return nil, fmt.Errorf("error creating output file: %w", err)
		}
		w.file = file
		w.writer = file
	}

	return w, nil
}

// Write writes content to the configured destination
func (w *Writer) Write(content string) error {
	_, err := fmt.Fprint(w.writer, content)
	return err
}

// WriteFormat writes formatted content
func (w *Writer) WriteFormat(format string, args ...interface{}) error {
	_, err := fmt.Fprintf(w.writer, format, args...)
	return err
}

// Close closes the writer if it's a file
func (w *Writer) Close() error {
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// MergeJSONResults combines multiple JSON responses from the API into a single JSON object
func MergeJSONResults(results []string) (string, error) {
	if len(results) == 0 {
		return "{}", nil
	}

	// If there's only one result, return it directly
	if len(results) == 1 {
		return results[0], nil
	}

	// Map to combine all results
	merged := make(map[string]interface{})

	// Parse each JSON and merge
	for _, result := range results {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(result), &data); err != nil {
			return "", fmt.Errorf("error parsing JSON: %w", err)
		}

		// Merge fields into the merged map
		for key, value := range data {
			merged[key] = value
		}
	}

	// Convert back to JSON
	output, err := json.Marshal(merged)
	if err != nil {
		return "", fmt.Errorf("error marshaling merged JSON: %w", err)
	}

	return string(output), nil
}
