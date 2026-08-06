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

package wfp

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/scanoss/scanoss.go/pkg/filter"
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

	fp := Files([]string{tiny, big}, 2, root, nil)
	if len(fp.Errors) > 0 {
		t.Fatalf("Generate errors = %v", fp.Errors)
	}
	for _, want := range []string{"tiny.go", "big.go"} {
		if !strings.Contains(string(fp.WFP), want) {
			t.Errorf("WFP missing %q; got:\n%s", want, fp.WFP)
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

	fp := Files([]string{hidden, visible}, 1, root, nil)
	if len(fp.Errors) > 0 {
		t.Fatalf("Generate errors = %v", fp.Errors)
	}
	if !strings.Contains(string(fp.WFP), ".secret.go") {
		t.Errorf("WFP should contain the hidden file when handed in directly; got:\n%s", fp.WFP)
	}
	if !strings.Contains(string(fp.WFP), "main.go") {
		t.Errorf("WFP missing main.go; got:\n%s", fp.WFP)
	}
}

// The fingerprint layer applies no filtering of its own: what is worth
// fingerprinting is decided once, during collection. Handing it a file the
// default lists would have excluded fingerprints it, because the caller asked.
//
// This is the contract change of the unification: the layer stops second-
// guessing its input. Callers that want the rules apply them first, via
// filter.Collect, or compose one from filter's exported sources.
func TestGenerateWFPDoesNotFilter(t *testing.T) {
	root := t.TempDir()
	png := filepath.Join(root, "logo.png")
	writeSized(t, png, 400)

	fp := Files([]string{png}, 1, root, nil)
	if len(fp.Errors) > 0 {
		t.Fatalf("Generate errors = %v", fp.Errors)
	}
	if !strings.Contains(string(fp.WFP), "logo.png") {
		t.Errorf("logo.png should be fingerprinted when handed in directly; got:\n%s", fp.WFP)
	}
}

// And the collection still excludes it, so the CLI paths are unchanged: the
// file never reaches the worker in the first place.
func TestCollectionStillExcludesWhatTheLayerNoLongerDoes(t *testing.T) {
	root := t.TempDir()
	writeSized(t, filepath.Join(root, "logo.png"), 400)
	writeSized(t, filepath.Join(root, "main.go"), 400)

	res, err := filter.Collect(root, filter.Scanning(nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Files {
		if filepath.Base(f) == "logo.png" {
			t.Error("collection must still exclude logo.png")
		}
	}
}

// The WFP is byte-reproducible: completion order depends on worker scheduling,
// so the output is sorted by path, whatever the thread count.
func TestFilesOutputSortedByPath(t *testing.T) {
	root := t.TempDir()
	var paths []string
	for _, name := range []string{"zebra.go", "alpha.go", "mid.go", "beta.go", "omega.go", "delta.go", "kappa.go", "gamma.go"} {
		p := filepath.Join(root, name)
		writeSized(t, p, 100+len(name)*30) // varied sizes shuffle completion order
		paths = append(paths, p)
	}

	first := Files(paths, 8, root, nil)
	if len(first.Errors) > 0 {
		t.Fatalf("errors = %v", first.Errors)
	}

	var got []string
	for _, line := range strings.Split(string(first.WFP), "\n") {
		if rest, ok := strings.CutPrefix(line, "file="); ok {
			parts := strings.SplitN(rest, ",", 3)
			got = append(got, parts[2])
		}
	}
	want := []string{"alpha.go", "beta.go", "delta.go", "gamma.go", "kappa.go", "mid.go", "omega.go", "zebra.go"}
	if !slices.Equal(got, want) {
		t.Errorf("WFP file order = %v, want %v", got, want)
	}
	for i, fp := range first.Files {
		if fp.Path != want[i] {
			t.Errorf("Files[%d].Path = %q, want %q", i, fp.Path, want[i])
		}
	}

	second := Files(paths, 8, root, nil)
	if !slices.Equal(first.WFP, second.WFP) {
		t.Error("two runs over the same files produced different WFP bytes")
	}
}

// A directory handed to Files is reported, not silently skipped: the caller
// built the list, so an entry that cannot be fingerprinted is its mistake to hear about.
func TestFilesReportsDirectories(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "somedir")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "main.go")
	writeSized(t, file, 200)

	fp := Files([]string{dir, file}, 2, root, nil)
	if len(fp.Errors) != 1 || !strings.Contains(fp.Errors[0].Error(), "somedir") {
		t.Errorf("Errors = %v, want one naming somedir", fp.Errors)
	}
	if !strings.Contains(string(fp.WFP), "main.go") {
		t.Errorf("the sibling file must still be fingerprinted; got:\n%s", fp.WFP)
	}
}

// Progress counts failures too: a run with unreadable files still ends at
// done == total, so a progress bar fed by the callback completes.
func TestFilesProgressCountsFailures(t *testing.T) {
	root := t.TempDir()
	good := filepath.Join(root, "good.go")
	writeSized(t, good, 200)
	missing := filepath.Join(root, "gone.go")

	var last int
	fp := Files([]string{good, missing}, 2, root, func(done, total int) {
		last = done
		if total != 2 {
			t.Errorf("total = %d, want 2", total)
		}
	})
	if last != 2 {
		t.Errorf("final done = %d, want 2 (failures must advance progress)", last)
	}
	if len(fp.Errors) != 1 {
		t.Errorf("Errors = %v, want one for the missing file", fp.Errors)
	}
}

// Folder filters and Files does not: the difference is the whole reason both exist. The same
// directory through each yields a fingerprinted .md from Files and none from Folder.
func TestFolderFiltersAndFilesDoesNot(t *testing.T) {
	root := t.TempDir()
	code := filepath.Join(root, "code.c")
	notes := filepath.Join(root, "notes.md")
	writeSized(t, code, 200)
	writeSized(t, notes, 200)

	folder, err := Folder(root, nil, 1, nil)
	if err != nil {
		t.Fatalf("Folder: %v", err)
	}
	if len(folder.Files) != 1 || filepath.Base(folder.Files[0].Path) != "code.c" {
		t.Errorf("Folder kept %d file(s) (%+v), want only code.c", len(folder.Files), folder.Files)
	}
	if folder.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 — the .md the filters excluded", folder.Skipped)
	}

	files := Files([]string{code, notes}, 1, root, nil)
	if len(files.Files) != 2 {
		t.Errorf("Files kept %d file(s), want both: it selects nothing", len(files.Files))
	}
	if files.Skipped != 0 {
		t.Errorf("Skipped = %d, want 0 from Files", files.Skipped)
	}
}

// A nil filters means the fingerprinting profile, so a caller with a directory and no opinion
// gets the rules a scan would apply.
func TestFolderNilFiltersUsesTheFingerprintingProfile(t *testing.T) {
	root := t.TempDir()
	writeSized(t, filepath.Join(root, "keep.c"), 200)
	writeSized(t, filepath.Join(root, "drop.json"), 200)

	res, err := Folder(root, nil, 1, nil)
	if err != nil {
		t.Fatalf("Folder: %v", err)
	}
	if len(res.Files) != 1 || filepath.Base(res.Files[0].Path) != "keep.c" {
		t.Errorf("collected %+v, want only keep.c (.json is in the built-in skip list)", res.Files)
	}
}
