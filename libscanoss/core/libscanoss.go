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

package main

import "C"
import (
	"encoding/json"

	"github.com/scanoss/scanoss.go/internal/version"
	"github.com/scanoss/scanoss.go/pkg/api"
	"github.com/scanoss/scanoss.go/pkg/batch"
	wfpPkg "github.com/scanoss/scanoss.go/pkg/fingerprint/wfp"
	"github.com/scanoss/scanoss.go/pkg/output"
	"github.com/scanoss/scanoss.go/pkg/scanner"
)

// GenerateWFP generates a WFP fingerprint for a single file
//
//export GenerateWFP
func GenerateWFP(filePath *C.char) *C.char {
	goFilePath := C.GoString(filePath)

	fp, err := wfpPkg.GenerateFingerprint(goFilePath, "")
	if err != nil {
		return C.CString("")
	}

	return C.CString(fp.Fingerprint)
}

// GenerateWFPJSON generates a WFP fingerprint and returns full JSON metadata
//
//export GenerateWFPJSON
func GenerateWFPJSON(filePath *C.char) *C.char {
	goFilePath := C.GoString(filePath)

	fp, err := wfpPkg.GenerateFingerprint(goFilePath, "")
	if err != nil {
		errJSON, _ := json.Marshal(map[string]string{"error": err.Error()})
		return C.CString(string(errJSON))
	}

	jsonData, err := json.Marshal(fp)
	if err != nil {
		errJSON, _ := json.Marshal(map[string]string{"error": err.Error()})
		return C.CString(string(errJSON))
	}

	return C.CString(string(jsonData))
}

// CollectFiles collects all valid files from a directory
//
//export CollectFiles
func CollectFiles(path *C.char) *C.char {
	goPath := C.GoString(path)

	files, err := scanner.CollectFiles(goPath)
	if err != nil {
		errJSON, _ := json.Marshal(map[string]string{"error": err.Error()})
		return C.CString(string(errJSON))
	}

	jsonData, err := json.Marshal(files)
	if err != nil {
		errJSON, _ := json.Marshal(map[string]string{"error": err.Error()})
		return C.CString(string(errJSON))
	}

	return C.CString(string(jsonData))
}

// Scan performs a complete scan: collects files, generates fingerprints, and sends to API
//
//export Scan
func Scan(path *C.char, apiURL *C.char, apiKey *C.char, threads C.int, postSize C.int) *C.char {
	goPath := C.GoString(path)
	goAPIURL := C.GoString(apiURL)
	goAPIKey := C.GoString(apiKey)
	goThreads := int(threads)
	goPostSize := int(postSize)

	// Collect files
	files, err := scanner.CollectFiles(goPath)
	if err != nil {
		errJSON, _ := json.Marshal(map[string]string{"error": err.Error()})
		return C.CString(string(errJSON))
	}

	if len(files) == 0 {
		errJSON, _ := json.Marshal(map[string]string{"error": "no valid files found"})
		return C.CString(string(errJSON))
	}

	// Create worker pool
	pool := scanner.NewWorkerPool(goThreads)
	pool.Start()

	// Create batcher
	batcher := batch.NewStreamingBatcher(goPostSize)

	// Create API client
	apiClient := api.NewClient(goAPIURL, goAPIKey)

	// Submit jobs
	go func() {
		for _, file := range files {
			pool.Submit(file)
		}
		pool.Close()
	}()

	// Collect results and batch
	go func() {
		for fp := range pool.Results() {
			batcher.Add(fp)
		}
		batcher.Close()
	}()

	// Send batches and collect responses
	var allResults []string
	for batchWithMeta := range batcher.Batches() {
		wfpContent := batch.CombineFingerprints(batchWithMeta.Data)
		response, err := apiClient.SendBatch(wfpContent)
		if err != nil {
			// Skip errors, continue with next batch
			continue
		}
		allResults = append(allResults, response)
	}

	// Merge all results
	mergedResult, err := output.MergeJSONResults(allResults)
	if err != nil {
		errJSON, _ := json.Marshal(map[string]string{"error": err.Error()})
		return C.CString(string(errJSON))
	}

	return C.CString(mergedResult)
}

// GetVersion returns the library version, single-sourced from the git tag
// (the same value the CLI reports; see internal/version).
//
//export GetVersion
func GetVersion() *C.char {
	return C.CString(version.Version())
}

func main() {}
