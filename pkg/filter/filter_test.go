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
	"testing"
	"time"
)

// fakeInfo is a minimal os.FileInfo for matcher unit tests.
type fakeInfo struct {
	name  string
	size  int64
	isDir bool
}

func (f fakeInfo) Name() string { return f.name }
func (f fakeInfo) Size() int64  { return f.size }
func (f fakeInfo) Mode() os.FileMode {
	if f.isDir {
		return os.ModeDir
	}
	return 0
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
	writeFile(t, filepath.Join(root, "small.go"), 50)              // skip: min size
	writeFile(t, filepath.Join(root, "README"), 200)               // skip: ending
	writeFile(t, filepath.Join(root, "Makefile"), 200)             // skip: name
	writeFile(t, filepath.Join(root, "node_modules", "x.js"), 200) // skip: dir
	writeFile(t, filepath.Join(root, "__pycache__", "y.go"), 200)  // skip: dir
	writeFile(t, filepath.Join(root, "vendor", "v.go"), 200)       // skip: dir

	res, err := Collect(root, Options{Defaults: true})
	if err != nil {
		t.Fatal(err)
	}
	got := baseNames(res.Files)
	want := []string{"main.go"}
	if !equalStrings(got, want) {
		t.Fatalf("kept files = %v, want %v", got, want)
	}
	// a.png, b.md, small.go, README, Makefile were visited and skipped (dir
	// contents are pruned, not counted).
	if res.SkippedCount != 5 {
		t.Fatalf("SkippedCount = %d, want 5", res.SkippedCount)
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
