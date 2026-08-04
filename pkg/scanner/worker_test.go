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

package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSized creates a file of exactly size bytes of printable content.
func writeSized(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.Repeat("a", size)), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The worker applies no size bound of its own: size policy belongs to
// collection, which is the only stage that reports what it skipped. A small file
// handed to the pool must be fingerprinted, not silently dropped.
func TestGenerateWFPKeepsSmallFiles(t *testing.T) {
	root := t.TempDir()
	tiny := filepath.Join(root, "tiny.go")
	big := filepath.Join(root, "big.go")
	writeSized(t, tiny, 40)
	writeSized(t, big, 200)

	wfp, errs := GenerateWFP([]string{tiny, big}, 2, root, nil)
	if len(errs) > 0 {
		t.Fatalf("GenerateWFP errors = %v", errs)
	}
	for _, want := range []string{"tiny.go", "big.go"} {
		if !strings.Contains(string(wfp), want) {
			t.Errorf("WFP missing %q; got:\n%s", want, wfp)
		}
	}
}

// Hidden files are decided during collection, not here: the worker applies no
// policy of its own. Handing it a dotfile fingerprints it, which is what makes
// --all-hidden possible at all.
func TestGenerateWFPDoesNotDecideOnHiddenFiles(t *testing.T) {
	root := t.TempDir()
	hidden := filepath.Join(root, ".secret.go")
	visible := filepath.Join(root, "main.go")
	writeSized(t, hidden, 200)
	writeSized(t, visible, 200)

	wfp, errs := GenerateWFP([]string{hidden, visible}, 1, root, nil)
	if len(errs) > 0 {
		t.Fatalf("GenerateWFP errors = %v", errs)
	}
	if !strings.Contains(string(wfp), ".secret.go") {
		t.Errorf("WFP should contain the hidden file when handed in directly; got:\n%s", wfp)
	}
	if !strings.Contains(string(wfp), "main.go") {
		t.Errorf("WFP missing main.go; got:\n%s", wfp)
	}
}

// The fingerprint layer applies no filtering of its own: what is worth
// fingerprinting is decided once, during collection. Handing it a file the
// default lists would have excluded fingerprints it, because the caller asked.
//
// This is the contract change of the unification: the layer stops second-
// guessing its input. Callers that want the rules apply them first, via
// scanner.CollectFilesWithOptions, or compose one from filter's exported sources.
func TestGenerateWFPDoesNotFilter(t *testing.T) {
	root := t.TempDir()
	png := filepath.Join(root, "logo.png")
	writeSized(t, png, 400)

	wfp, errs := GenerateWFP([]string{png}, 1, root, nil)
	if len(errs) > 0 {
		t.Fatalf("GenerateWFP errors = %v", errs)
	}
	if !strings.Contains(string(wfp), "logo.png") {
		t.Errorf("logo.png should be fingerprinted when handed in directly; got:\n%s", wfp)
	}
}

// And the collection still excludes it, so the CLI paths are unchanged: the
// file never reaches the worker in the first place.
func TestCollectionStillExcludesWhatTheLayerNoLongerDoes(t *testing.T) {
	root := t.TempDir()
	writeSized(t, filepath.Join(root, "logo.png"), 400)
	writeSized(t, filepath.Join(root, "main.go"), 400)

	files, err := CollectFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if filepath.Base(f) == "logo.png" {
			t.Error("collection must still exclude logo.png")
		}
	}
}
