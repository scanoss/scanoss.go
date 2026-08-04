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
	"context"
	"encoding/json"
	"errors"

	"github.com/scanoss/scanoss.go/internal/version"
	"github.com/scanoss/scanoss.go/pkg/filter"
	"github.com/scanoss/scanoss.go/pkg/scanoss"
	"github.com/scanoss/scanoss.go/pkg/wfp"
)

// errorJSON renders err in the shape every export uses to report a failure.
func errorJSON(err error) *C.char {
	payload, _ := json.Marshal(map[string]string{"error": err.Error()})
	return C.CString(string(payload))
}

// skipFile reports whether the built-in rules exclude path. Collect takes a single file
// as its root, so these entry points ask it directly instead of restating the rules —
// one answer, from the same code a scan uses.
func skipFile(path string) bool {
	res, err := filter.Collect(path, filter.Scanning(nil))
	if err != nil {
		return false // let the fingerprinter report the real error
	}
	return len(res.Files) == 0
}

// GenerateWFP generates a WFP fingerprint for a single file
//
//export GenerateWFP
func GenerateWFP(filePath *C.char) *C.char {
	goFilePath := C.GoString(filePath)
	if skipFile(goFilePath) {
		return C.CString("")
	}

	res := wfp.Generate([]string{goFilePath}, 1, "", nil)
	if len(res.Files) == 0 {
		return C.CString("")
	}

	return C.CString(res.Files[0].Fingerprint)
}

// GenerateWFPJSON generates a WFP fingerprint and returns full JSON metadata
//
//export GenerateWFPJSON
func GenerateWFPJSON(filePath *C.char) *C.char {
	goFilePath := C.GoString(filePath)
	if skipFile(goFilePath) {
		errJSON, _ := json.Marshal(map[string]string{"error": "file extension filtered"})
		return C.CString(string(errJSON))
	}

	res := wfp.Generate([]string{goFilePath}, 1, "", nil)
	if len(res.Files) == 0 {
		if len(res.Errors) > 0 {
			return errorJSON(res.Errors[0])
		}
		return errorJSON(errors.New("could not fingerprint the file"))
	}

	jsonData, err := json.Marshal(res.Files[0])
	if err != nil {
		return errorJSON(err)
	}

	return C.CString(string(jsonData))
}

// CollectFiles collects all valid files from a directory
//
//export CollectFiles
func CollectFiles(path *C.char) *C.char {
	goPath := C.GoString(path)

	res, err := filter.Collect(goPath, filter.Scanning(nil))
	if err != nil {
		return errorJSON(err)
	}

	jsonData, err := json.Marshal(res.Files)
	if err != nil {
		return errorJSON(err)
	}

	return C.CString(string(jsonData))
}

// Scan performs a complete scan of path: collects files, fingerprints them with threads
// workers, uploads the WFP in postSize-byte chunks and waits for the report.
//
// Unlike the hand-rolled pipeline this replaced, a failed upload is now an error rather
// than a skipped chunk, so a partial scan no longer reports success.
//
//export Scan
func Scan(path *C.char, apiURL *C.char, apiKey *C.char, threads C.int, postSize C.int) *C.char {
	client, err := scanoss.New(scanoss.Config{
		APIURL:  C.GoString(apiURL),
		APIKey:  C.GoString(apiKey),
		Workers: int(threads),
	})
	if err != nil {
		return errorJSON(err)
	}

	env, err := client.Scan.Folder(context.Background(), C.GoString(path),
		scanoss.WithChunkBytes(int(postSize)))
	if err != nil {
		return errorJSON(err)
	}

	result, err := json.Marshal(env.Result)
	if err != nil {
		return errorJSON(err)
	}
	return C.CString(string(result))
}

// GetVersion returns the library version, single-sourced from the git tag
// (the same value the CLI reports; see internal/version).
//
//export GetVersion
func GetVersion() *C.char {
	return C.CString(version.Version())
}

func main() {}
