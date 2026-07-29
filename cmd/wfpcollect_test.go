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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTreeFile creates a file of size bytes, making parent dirs as needed.
func writeTreeFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("a", size)), 0o644); err != nil {
		t.Fatal(err)
	}
}

// runWFPTree runs the real wfp command over root and returns the WFP it wrote.
// Every collection flag is passed explicitly: cobra keeps flag values between
// Execute calls in one process, so an omitted flag would inherit the last run's.
func runWFPTree(t *testing.T, root string, defaults, gitignore string) string {
	t.Helper()
	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true

	out := filepath.Join(t.TempDir(), "out.wfp")
	rootCmd.SetArgs([]string{
		"wfp", root, "--output", out,
		"--min-size", "0", "--max-size", "0",
		"--default-filters=" + defaults, "--gitignore=" + gitignore,
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("wfp: %v", err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// --default-filters=false keeps a file the built-in skip lists would drop.
//
// The fixture is "Makefile" (dropped by the default file-name list) rather than
// something like "a.png", because pkg/fingerprint/wfp keeps its own copy of the
// skipped-extension list and re-applies it in the worker, after collection — so
// an extension-skipped file stays out of the WFP even with the defaults off.
// That second list is not something this command can override; see T011.
func TestWFPDefaultFiltersFlag(t *testing.T) {
	root := t.TempDir()
	writeTreeFile(t, filepath.Join(root, "main.go"), 200)
	writeTreeFile(t, filepath.Join(root, "Makefile"), 200)

	if got := runWFPTree(t, root, "true", "true"); strings.Contains(got, "Makefile") {
		t.Errorf("with default filters, Makefile should be skipped; got:\n%s", got)
	}
	if got := runWFPTree(t, root, "false", "true"); !strings.Contains(got, "Makefile") {
		t.Errorf("with --default-filters=false, Makefile should be kept; got:\n%s", got)
	}
}

// .gitignore is honoured, and can be switched off.
func TestWFPGitignoreFlag(t *testing.T) {
	root := t.TempDir()
	writeTreeFile(t, filepath.Join(root, "main.go"), 200)
	writeTreeFile(t, filepath.Join(root, "gen", "out.go"), 200)
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("gen/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := runWFPTree(t, root, "true", "true"); strings.Contains(got, "out.go") {
		t.Errorf("gen/out.go is gitignored and should be skipped; got:\n%s", got)
	}
	if got := runWFPTree(t, root, "true", "false"); !strings.Contains(got, "out.go") {
		t.Errorf("with --gitignore=false, gen/out.go should be kept; got:\n%s", got)
	}
}

// wfp reads scanoss.json's *fingerprinting* rules, not the scanning ones. If it
// used the wrong operation the two assertions below invert: skipme/ would be
// fingerprinted and keep/ would not.
func TestWFPUsesFingerprintingOperation(t *testing.T) {
	root := t.TempDir()
	writeTreeFile(t, filepath.Join(root, "keep", "keep.go"), 200)
	writeTreeFile(t, filepath.Join(root, "skipme", "skipme.go"), 200)
	cfg := `{
	  "settings": {
	    "skip": {
	      "patterns": {
	        "scanning":       ["keep/**"],
	        "fingerprinting": ["skipme/**"]
	      }
	    }
	  }
	}`
	if err := os.WriteFile(filepath.Join(root, "scanoss.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	got := runWFPTree(t, root, "true", "true")
	if strings.Contains(got, "skipme.go") {
		t.Errorf("skipme/ is skipped for fingerprinting and must not appear; got:\n%s", got)
	}
	if !strings.Contains(got, "keep.go") {
		t.Errorf("keep/ is only skipped for scanning, so wfp must keep it; got:\n%s", got)
	}
}
