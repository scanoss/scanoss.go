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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

// sizeFixture builds the verification tree: files straddling both bounds of a
// scanoss.json rule scoped to **/*.ts, plus a same-sized file outside those
// patterns. The rule is declared for both operations so `scan` (scanning) and
// `wfp` (fingerprinting) see the same thing.
func sizeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeTreeFile(t, filepath.Join(root, "tiny.ts"), 50)      // below the rule's min
	writeTreeFile(t, filepath.Join(root, "ok.ts"), 500)       // inside the rule
	writeTreeFile(t, filepath.Join(root, "huge.ts"), 1200000) // above the rule's max
	writeTreeFile(t, filepath.Join(root, "tiny.rs"), 50)      // outside the rule's patterns

	rule := `[{ "patterns": ["**/*.ts"], "min": 100, "max": 1048576 }]`
	cfg := `{"settings":{"skip":{"sizes":{"scanning":` + rule + `,"fingerprinting":` + rule + `}}}}`
	if err := os.WriteFile(filepath.Join(root, "scanoss.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// wfpFiles returns the sorted "file=" names in a WFP.
func wfpFiles(wfp string) []string {
	var out []string
	for _, line := range strings.Split(wfp, "\n") {
		if !strings.HasPrefix(line, "file=") {
			continue
		}
		parts := strings.Split(line, ",")
		out = append(out, parts[len(parts)-1])
	}
	sort.Strings(out)
	return out
}

// A scoped scanoss.json rule bounds only the files its patterns match, and the
// global flag bounds everything. tiny.rs is the load-bearing case: it is as
// small as tiny.ts but outside the rule, so nothing may drop it by default.
func TestSizeBoundsEndToEnd(t *testing.T) {
	root := sizeFixture(t)

	tests := []struct {
		name     string
		min, max string
		want     []string
	}{
		{"no global bound", "0", "0", []string{"ok.ts", "tiny.rs"}},
		{"global minimum", "100", "0", []string{"ok.ts"}},
		{"global window excludes everything", "100", "150", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rootCmd.SetOut(io.Discard)
			rootCmd.SetErr(io.Discard)
			rootCmd.SilenceUsage = true
			rootCmd.SilenceErrors = true

			out := filepath.Join(t.TempDir(), "out.wfp")
			rootCmd.SetArgs([]string{
				"wfp", root, "--output", out,
				"--min-size", tc.min, "--max-size", tc.max,
				"--default-filters=true", "--gitignore=true",
			})
			if err := rootCmd.Execute(); err != nil {
				t.Fatalf("wfp: %v", err)
			}
			b, err := os.ReadFile(out)
			if err != nil && len(tc.want) > 0 {
				t.Fatal(err)
			}
			got := wfpFiles(string(b))
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("kept %v, want %v", got, tc.want)
			}
		})
	}
}

// wfp and scan must cover the same files for the same flags. The scan's upload
// body is the WFP it would send, so capturing it compares the two directly; the
// scan then fails on the fake 500, which is expected and irrelevant here.
func TestWFPAndScanCoverTheSameFiles(t *testing.T) {
	root := sizeFixture(t)

	var mu sync.Mutex
	var uploaded strings.Builder
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if body, err := io.ReadAll(r.Body); err == nil {
			mu.Lock()
			uploaded.Write(body)
			mu.Unlock()
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true

	out := filepath.Join(t.TempDir(), "out.wfp")
	rootCmd.SetArgs([]string{
		"wfp", root, "--output", out,
		"--min-size", "0", "--max-size", "0",
		"--default-filters=true", "--gitignore=true",
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("wfp: %v", err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	fromWFP := wfpFiles(string(b))

	// The scan is expected to fail against the fake server; only its upload matters.
	rootCmd.SetArgs([]string{
		"scan", root,
		"--min-size", "0", "--max-size", "0",
		"--default-filters=true", "--gitignore=true",
		"--api-url", srv.URL, "--api-key", "test",
	})
	_ = rootCmd.Execute()

	mu.Lock()
	fromScan := wfpFiles(uploaded.String())
	mu.Unlock()

	if len(fromScan) == 0 {
		t.Fatal("the scan uploaded nothing; the comparison would be vacuous")
	}
	if strings.Join(fromWFP, ",") != strings.Join(fromScan, ",") {
		t.Fatalf("wfp covered %v but scan uploaded %v", fromWFP, fromScan)
	}
}
