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

package filter

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// fakeInfo is a minimal os.FileInfo for matcher unit tests.
type fakeInfo struct {
	name  string
	size  int64
	isDir bool
	mode  os.FileMode
}

func (f fakeInfo) Name() string { return f.name }
func (f fakeInfo) Size() int64  { return f.size }
func (f fakeInfo) Mode() os.FileMode {
	if f.isDir {
		return os.ModeDir | f.mode
	}
	return f.mode
}
func (f fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool        { return f.isDir }
func (f fakeInfo) Sys() any           { return nil }

func aFile(name string, size int64) fakeInfo { return fakeInfo{name: name, size: size} }
func aDir(name string) fakeInfo              { return fakeInfo{name: name, isDir: true} }

func TestLeafMatchers(t *testing.T) {
	tests := []struct {
		name    string
		matcher Matcher
		rel     string
		info    fakeInfo
		want    bool
	}{
		{"ext matches file", newExtMatcher(".png"), "a/b.png", aFile("b.png", 10), true},
		{"ext ignores dir", newExtMatcher(".png"), "b.png", aDir("b.png"), false},
		{"ext case-insensitive", newExtMatcher(".png"), "B.PNG", aFile("B.PNG", 10), true},
		{"compound ext", newExtMatcher(".min.js"), "x.min.js", aFile("x.min.js", 10), true},
		{"ext no match", newExtMatcher(".png"), "main.go", aFile("main.go", 10), false},
		{"name exact", newNameMatcher("makefile"), "makefile", aFile("Makefile", 10), true},
		{"name no match", newNameMatcher("makefile"), "main.go", aFile("main.go", 10), false},
		{"dir name", newDirNameMatcher("vendor"), "vendor", aDir("vendor"), true},
		{"dir name ignores file", newDirNameMatcher("vendor"), "vendor", aFile("vendor", 10), false},
		{"dir suffix", newDirSuffixMatcher(".egg-info"), "foo.egg-info", aDir("foo.egg-info"), true},
		{"ending", newEndingMatcher("readme"), "README", aFile("README", 10), true},
		{"size below min", newSizeMatcher(100, 0), "a", aFile("a", 50), true},
		{"size within", newSizeMatcher(100, 0), "a", aFile("a", 150), false},
		{"size above max", newSizeMatcher(0, 1000), "a", aFile("a", 2000), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.matcher.Match(tc.rel, tc.info); got != tc.want {
				t.Fatalf("Match(%q) = %v, want %v", tc.rel, got, tc.want)
			}
		})
	}
}

func TestBuildDedupe(t *testing.T) {
	c := Build(
		[]Matcher{newExtMatcher(".png")},
		[]Matcher{newExtMatcher(".png"), newExtMatcher(".gif")},
	)
	if got := len(c.Matchers()); got != 2 {
		t.Fatalf("deduped matcher count = %d, want 2", got)
	}
}

// writeFile creates a file of exactly size bytes, making parent dirs as needed.
func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func baseNames(files []string) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, filepath.Base(f))
	}
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestCollectDefaults(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.go"), 200)              // keep
	writeFile(t, filepath.Join(root, "a.png"), 200)                // skip: ext
	writeFile(t, filepath.Join(root, "b.md"), 200)                 // skip: ext
	writeFile(t, filepath.Join(root, "small.go"), 50)              // keep: no minimum by default
	writeFile(t, filepath.Join(root, "README"), 200)               // skip: ending
	writeFile(t, filepath.Join(root, "Makefile"), 200)             // skip: name
	writeFile(t, filepath.Join(root, "node_modules", "x.js"), 200) // skip: dir
	writeFile(t, filepath.Join(root, "__pycache__", "y.go"), 200)  // skip: dir
	writeFile(t, filepath.Join(root, "vendor", "v.go"), 200)       // skip: dir

	res, err := Collect(root, Options{FolderDefaults: true, FileDefaults: true})
	if err != nil {
		t.Fatal(err)
	}
	got := baseNames(res.Files)
	want := []string{"main.go", "small.go"}
	if !equalStrings(got, want) {
		t.Fatalf("kept files = %v, want %v", got, want)
	}
	// a.png, b.md, README, Makefile were visited and skipped (dir contents are
	// pruned, not counted).
	if res.SkippedCount != 4 {
		t.Fatalf("SkippedCount = %d, want 4", res.SkippedCount)
	}
}

// Zero-byte files are always skipped: they carry no content to match, so a
// fingerprint of one is a WFP entry with a zero hash and no lines. The rule does
// not depend on the defaults, on the size bounds, or on anything a caller can
// switch off.
func TestCollectSkipsUnscannable(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "empty.go"), 0)
	writeFile(t, filepath.Join(root, "main.go"), 200)

	for _, o := range map[string]Options{
		"defaults on":            {FolderDefaults: true, FileDefaults: true},
		"defaults off":           {FolderDefaults: false, FileDefaults: false},
		"zero-valued options":    {},
		"with an explicit floor": {FolderDefaults: true, FileDefaults: true, MinSize: 100},
	} {
		res, err := Collect(root, o)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range res.Files {
			if filepath.Base(f) == "empty.go" {
				t.Errorf("%+v: empty.go must never be collected", o)
			}
		}
		if res.SkippedCount < 1 {
			t.Errorf("%+v: the empty file must be counted as skipped", o)
		}
	}
}

// A symlink is skipped: its target is collected on its own when it is inside the
// tree, so following it would report the same content twice under two names.
// Both scanoss.py and scanoss.js drop them too.
func TestCollectSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "real.go"), 200)
	if err := os.Symlink(filepath.Join(root, "real.go"), filepath.Join(root, "link.go")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	res, err := Collect(root, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if got := baseNames(res.Files); !equalStrings(got, []string{"real.go"}) {
		t.Fatalf("kept %v, want [real.go] — the link must not be collected", got)
	}
	if res.SkippedCount != 1 {
		t.Errorf("SkippedCount = %d, want 1", res.SkippedCount)
	}
}

// A matcher composed the way an external caller composes one (Build over the
// exported sources) agrees with Collect on unscannable entries.
func TestComposedMatcherSkipsUnscannable(t *testing.T) {
	for _, o := range []Options{{FolderDefaults: true, FileDefaults: true}, {FolderDefaults: false, FileDefaults: false}, {}} {
		m := Build(UnscannableSource(), DefaultSource(o.defaults()))
		if !m.Match("empty.go", aFile("empty.go", 0)) {
			t.Errorf("%+v: a zero-byte file should be skipped", o)
		}
		if !m.Match("link.go", fakeInfo{name: "link.go", size: 20, mode: os.ModeSymlink}) {
			t.Errorf("%+v: a symlink should be skipped", o)
		}
		if m.Match("main.go", aFile("main.go", 200)) {
			t.Errorf("%+v: a 200-byte file should not be skipped", o)
		}
	}
}

// The size bounds are the caller's input, not part of the built-in lists, so
// switching the defaults off must not discard them.
func TestCollectSizeBoundsIndependentOfDefaults(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "tiny.go"), 40)
	writeFile(t, filepath.Join(root, "big.go"), 200)

	for _, useDefaults := range []bool{true, false} {
		res, err := Collect(root, Options{FolderDefaults: useDefaults, FileDefaults: useDefaults, MinSize: 100})
		if err != nil {
			t.Fatal(err)
		}
		if got := baseNames(res.Files); !equalStrings(got, []string{"big.go"}) {
			t.Errorf("Defaults=%v: kept %v, want [big.go]", useDefaults, got)
		}
		if res.SkippedCount != 1 {
			t.Errorf("Defaults=%v: SkippedCount = %d, want 1", useDefaults, res.SkippedCount)
		}
	}
}

// Size bounds are their own source, so a caller composing by hand gets them
// whether or not it also asked for the built-in lists.
func TestComposedSizeBoundsIndependentOfDefaults(t *testing.T) {
	for _, useDefaults := range []bool{true, false} {
		o := Options{FolderDefaults: useDefaults, FileDefaults: useDefaults, MinSize: 100}
		var srcs [][]Matcher
		if useDefaults {
			srcs = append(srcs, DefaultSource(o.defaults()))
		}
		srcs = append(srcs, SizeSource(o.MinSize, o.MaxSize))
		m := Build(srcs...)
		if !m.Match("tiny.go", aFile("tiny.go", 40)) {
			t.Errorf("Defaults=%v: a 40-byte file should be skipped by MinSize=100", useDefaults)
		}
		if m.Match("big.go", aFile("big.go", 200)) {
			t.Errorf("Defaults=%v: a 200-byte file should not be skipped", useDefaults)
		}
	}
}

// Options carries the bounds as plain int64, so a zero field is indistinguishable
// from a deliberate 0. That is harmless only while the built-in default *is* 0:
// an Options built literally then behaves exactly like one from a constructor.
//
// If this fails, someone gave the package a non-zero default bound, and the two
// construction paths have silently diverged — a literal Options{} would impose no
// bound while ScanOptions() imposes one. Fix it by making the fields *int64 (so
// "unset" and "zero" are distinct) rather than by relaxing this test.
func TestZeroOptionsMatchDefaultBounds(t *testing.T) {
	if DefaultMinFileSize != 0 {
		t.Errorf("DefaultMinFileSize = %d: a non-zero default needs *int64 fields, see the comment above",
			DefaultMinFileSize)
	}
	if DefaultMaxFileSize != 0 {
		t.Errorf("DefaultMaxFileSize = %d: a non-zero default needs *int64 fields, see the comment above",
			DefaultMaxFileSize)
	}

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "tiny.go"), 40)

	viaLiteral, err := Collect(root, Options{FolderDefaults: true, FileDefaults: true, GitIgnore: true})
	if err != nil {
		t.Fatal(err)
	}
	viaConstructor, err := Collect(root, ScanOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !equalStrings(baseNames(viaLiteral.Files), baseNames(viaConstructor.Files)) {
		t.Fatalf("literal Options kept %v, ScanOptions kept %v: the two paths must agree",
			baseNames(viaLiteral.Files), baseNames(viaConstructor.Files))
	}
}

// The constructors carry the built-in bounds, so an SDK caller that starts from
// one gets the documented default rather than an implicit zero.
func TestOptionConstructorsCarryDefaultBounds(t *testing.T) {
	for name, o := range map[string]Options{
		"DefaultOptions":    DefaultOptions(),
		"ScanOptions":       ScanOptions(),
		"DependencyOptions": DependencyOptions(),
	} {
		if o.MinSize != DefaultMinFileSize {
			t.Errorf("%s().MinSize = %d, want DefaultMinFileSize (%d)", name, o.MinSize, DefaultMinFileSize)
		}
		if o.MaxSize != DefaultMaxFileSize {
			t.Errorf("%s().MaxSize = %d, want DefaultMaxFileSize (%d)", name, o.MaxSize, DefaultMaxFileSize)
		}
	}
}

// The default collection imposes no lower size bound; MinSize opts back into one.
func TestCollectMinSize(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "tiny.go"), 40)
	writeFile(t, filepath.Join(root, "big.go"), 200)

	res, err := Collect(root, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if got := baseNames(res.Files); !equalStrings(got, []string{"big.go", "tiny.go"}) {
		t.Fatalf("default kept files = %v, want [big.go tiny.go]", got)
	}
	if res.SkippedCount != 0 {
		t.Fatalf("default SkippedCount = %d, want 0", res.SkippedCount)
	}

	res, err = Collect(root, Options{FolderDefaults: true, FileDefaults: true, MinSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	if got := baseNames(res.Files); !equalStrings(got, []string{"big.go"}) {
		t.Fatalf("MinSize=100 kept files = %v, want [big.go]", got)
	}
	// The dropped file is reported, not silently discarded.
	if res.SkippedCount != 1 {
		t.Fatalf("MinSize=100 SkippedCount = %d, want 1", res.SkippedCount)
	}
}

func TestCollectScanossSettings(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "keep.go"), 200)        // keep
	writeFile(t, filepath.Join(root, "a.log"), 200)          // skip: pattern *.log
	writeFile(t, filepath.Join(root, "build", "x.log"), 200) // skip: pattern build/**
	writeFile(t, filepath.Join(root, "big.bin"), 200)        // skip: size rule
	writeFile(t, filepath.Join(root, "small.bin"), 50)       // keep: within size

	opts := Options{
		Settings: &Settings{
			Skip: Skip{
				Patterns: []string{"*.log", "build/**"},
				Sizes:    []SizeRule{{Patterns: []string{"*.bin"}, Max: 100}},
			},
		},
	}
	res, err := Collect(root, opts)
	if err != nil {
		t.Fatal(err)
	}
	got := baseNames(res.Files)
	want := []string{"keep.go", "small.bin"}
	if !equalStrings(got, want) {
		t.Fatalf("kept files = %v, want %v", got, want)
	}
}

func TestCollectSingleFile(t *testing.T) {
	// Regression: Collect must work when root is a file, not a directory.
	root := t.TempDir()
	// A sibling .gitignore must not cause "not a directory" errors.
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.tmp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fpath := filepath.Join(root, "main.go")
	writeFile(t, fpath, 200)

	res, err := Collect(fpath, DefaultOptions())
	if err != nil {
		t.Fatalf("Collect(single file) error: %v", err)
	}
	if got := baseNames(res.Files); !equalStrings(got, []string{"main.go"}) {
		t.Fatalf("kept files = %v, want [main.go]", got)
	}
}

func TestCollectGitIgnore(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".gitignore"), 0)
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored/\n*.tmp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "keep.go"), 200)         // keep
	writeFile(t, filepath.Join(root, "t.tmp"), 200)           // skip: *.tmp
	writeFile(t, filepath.Join(root, "ignored", "a.go"), 200) // skip: ignored/

	res, err := Collect(root, Options{GitIgnore: true})
	if err != nil {
		t.Fatal(err)
	}
	got := baseNames(res.Files)
	want := []string{"keep.go"}
	if !equalStrings(got, want) {
		t.Fatalf("kept files = %v, want %v", got, want)
	}
}

// Collect has the tree, so it prunes the directory and never looks inside. A
// caller without a tree owns that decision and has the exported lists for it.
func TestCollectPrunesExcludedDirectories(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "node_modules", "left-pad", "index.js"), 400)
	writeFile(t, filepath.Join(root, "src", "main.go"), 400)

	res, err := Collect(root, ScanOptions())
	if err != nil {
		t.Fatal(err)
	}
	if got := baseNames(res.Files); !equalStrings(got, []string{"main.go"}) {
		t.Fatalf("kept %v, want [main.go]", got)
	}
}

// Version-control metadata is excluded like any other dotted entry — by the hidden rule — and so
func TestVCSMetadataFollowsTheHiddenRule(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.go"), 400)
	for _, p := range []string{
		".git/config", ".git/objects/ab/cdef", ".git/HEAD",
		".svn/entries", ".hg/store/data", ".bzr/checkout/dirstate",
	} {
		writeFile(t, filepath.Join(root, filepath.FromSlash(p)), 400)
	}

	countVCS := func(o Options) int {
		res, err := Collect(root, o)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, f := range res.Files {
			rel, _ := filepath.Rel(root, f)
			for _, vcs := range []string{".git", ".svn", ".hg", ".bzr"} {
				if strings.HasPrefix(filepath.ToSlash(rel), vcs+"/") {
					n++
				}
			}
		}
		return n
	}

	// Hidden entries excluded — the default — keeps it all out, however the other rules are set.
	for name, o := range map[string]Options{
		"defaults on":         {FolderDefaults: true, FileDefaults: true, GitIgnore: true},
		"defaults off":        {FolderDefaults: false, FileDefaults: false},
		"zero-valued options": {},
	} {
		if n := countVCS(o); n != 0 {
			t.Errorf("%s: collected %d version-control entries, want none", name, n)
		}
	}

	// Hidden entries included reaches it, like any other dotfile.
	if n := countVCS(Options{FolderDefaults: true, FileDefaults: true, IncludeHidden: true}); n == 0 {
		t.Error("IncludeHidden collected no version-control entries: it should reach them like any other dotted entry")
	}
}
