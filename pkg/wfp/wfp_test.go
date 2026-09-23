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
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "update the WFP golden file")

// TestGenerateFingerprintGolden pins the exact WFP output for a fixed source file.
// It guards refactors of the fingerprint assembly against any byte-level change.
// Regenerate the golden with: go test ./pkg/wfp -run Golden -update
func TestGenerateFingerprintGolden(t *testing.T) {
	fp, err := generateFingerprint(filepath.Join("testdata", "sample.c"), "testdata", options{keepHeaders: true})
	if err != nil {
		t.Fatalf("generateFingerprint: %v", err)
	}

	golden := filepath.Join("testdata", "sample.wfp.golden")
	if *update {
		if err := os.WriteFile(golden, []byte(fp.Fingerprint), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden (run with -update to create it): %v", err)
	}
	if fp.Fingerprint != string(want) {
		t.Errorf("WFP output changed vs golden.\n--- got ---\n%s\n--- want ---\n%s", fp.Fingerprint, string(want))
	}
}

// The returned struct is what every later stage reads: Size is the byte count, Hash is
// the CRC64 as 16 hex digits, and the "file=" line repeats both.
func TestGenerateFingerprintFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.c")
	content := "int main(void) { return 0; }\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	fp, err := generateFingerprint(path, dir, options{keepHeaders: true})
	if err != nil {
		t.Fatalf("generateFingerprint: %v", err)
	}
	if fp.Size != len(content) {
		t.Errorf("Size = %d, want %d", fp.Size, len(content))
	}
	if len(fp.Hash) != 16 {
		t.Errorf("Hash = %q, want 16 hex digits", fp.Hash)
	}
	if want := fmt.Sprintf("file=%s,%d,a.c\n", fp.Hash, fp.Size); !strings.HasPrefix(fp.Fingerprint, want) {
		t.Errorf("first line = %q, want prefix %q", fp.Fingerprint, want)
	}
}

// root decides what the "file=" label says. The scan result reports these paths, so an
// empty root has to leave the path the caller passed untouched.
func TestGenerateFingerprintRootLabel(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src", "lib"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "src", "lib", "a.c")
	if err := os.WriteFile(path, []byte("int a;\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	for name, tc := range map[string]struct{ root, want string }{
		"relative to root":                   {dir, "src/lib/a.c"},
		"empty root keeps the path as given": {"", path},
		"root below the file still resolves": {filepath.Join(dir, "src"), "lib/a.c"},
	} {
		t.Run(name, func(t *testing.T) {
			fp, err := generateFingerprint(path, tc.root, options{keepHeaders: true})
			if err != nil {
				t.Fatalf("generateFingerprint: %v", err)
			}
			if fp.Path != tc.want {
				t.Errorf("Path = %q, want %q", fp.Path, tc.want)
			}
			if !strings.Contains(fp.Fingerprint, ","+tc.want+"\n") {
				t.Errorf("file= line does not carry %q:\n%s", tc.want, firstLine(fp.Fingerprint))
			}
		})
	}
}

// An empty file has no minutiae, but it still produces its file= line: collection
// decided it was worth fingerprinting, and this stage does not second-guess that.
func TestGenerateFingerprintEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.c")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	fp, err := generateFingerprint(path, dir, options{keepHeaders: true})
	if err != nil {
		t.Fatalf("generateFingerprint: %v", err)
	}
	if fp.Size != 0 {
		t.Errorf("Size = %d, want 0", fp.Size)
	}
	if lines := strings.Count(fp.Fingerprint, "\n"); lines != 1 {
		t.Errorf("want only the file= line, got %d lines:\n%s", lines, fp.Fingerprint)
	}
}

func TestGenerateFingerprintUnreadableFile(t *testing.T) {
	_, err := generateFingerprint(filepath.Join(t.TempDir(), "missing.c"), "", options{keepHeaders: true})
	if err == nil {
		t.Fatal("generateFingerprint succeeded on a missing file, want an error")
	}
	if !strings.Contains(err.Error(), "error reading file") {
		t.Errorf("error = %q, want it to name the read failure", err)
	}
}

// firstLine is the "file=" header, for failure messages that do not need the minutiae.
func firstLine(wfp string) string {
	if i := strings.IndexByte(wfp, '\n'); i >= 0 {
		return wfp[:i]
	}
	return wfp
}

// BenchmarkGenerateFingerprint measures fingerprinting a large source file — the
// per-file scan hot path. The fixture is big enough to exercise the fingerprint
// assembly (many winnowing minutiae).
func BenchmarkGenerateFingerprint(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "big.c")

	var buf strings.Builder
	for i := 0; i < 20000; i++ {
		fmt.Fprintf(&buf, "int compute_value_%d(int alpha, int beta) { return alpha * %d + beta - (alpha ^ %d); }\n", i, i, i%13)
	}
	if err := os.WriteFile(path, []byte(buf.String()), 0o644); err != nil {
		b.Fatalf("write fixture: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := generateFingerprint(path, "", options{keepHeaders: true}); err != nil {
			b.Fatalf("generateFingerprint: %v", err)
		}
	}
}

// bigSourceFile is long enough past its header to produce snippet minutiae on both sides of the
// offset, which is what makes the stripping observable.
func bigSourceFile() string {
	var sb strings.Builder
	sb.WriteString("/*\n * Copyright (c) 2026 Example Corp\n * Licensed under the MIT license\n */\n")
	sb.WriteString("#include <stdio.h>\n#include <stdlib.h>\n\n")
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&sb, "int compute_%d(int x) { return x * %d + 7; }\n", i, i)
	}
	return sb.String()
}

// With the header filter on the minutiae from the preamble are gone and the first surviving line is
// announced by a start_line marker — the shape scanoss.py emits, so a WFP from either client
// reads the same.
func TestSkipHeadersStripsPreambleAndMarksTheStart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.c")
	if err := os.WriteFile(path, []byte(bigSourceFile()), 0o644); err != nil {
		t.Fatal(err)
	}

	plain, err := generateFingerprint(path, dir, options{keepHeaders: true})
	if err != nil {
		t.Fatal(err)
	}
	stripped, err := generateFingerprint(path, dir, options{})
	if err != nil {
		t.Fatal(err)
	}

	offset := headerOffset(path, bigSourceFile(), 0)
	if offset <= 0 {
		t.Fatalf("fixture should have a header to strip, got offset %d", offset)
	}

	marker := fmt.Sprintf("start_line=%d\n", offset)
	if !strings.Contains(stripped.Fingerprint, marker) {
		t.Errorf("stripped WFP missing %q", marker)
	}
	if strings.Contains(plain.Fingerprint, "start_line=") {
		t.Error("an unfiltered WFP must not carry a start_line marker")
	}

	// No minutiae line may name a line inside the header any more.
	for _, line := range strings.Split(stripped.Fingerprint, "\n") {
		num, _, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		n, convErr := strconv.Atoi(num)
		if convErr != nil {
			continue // file= or start_line=
		}
		if n <= offset {
			t.Errorf("line %d survived, but the header ends at %d", n, offset)
		}
	}

	// The file= line states what was read, not what was fingerprinted: same hash, same size.
	if stripped.Hash != plain.Hash || stripped.Size != plain.Size {
		t.Errorf("file= line changed: %s/%d vs %s/%d",
			stripped.Hash, stripped.Size, plain.Hash, plain.Size)
	}
}

// A file whose language the filter does not know is fingerprinted whole even with the option on.
func TestSkipHeadersLeavesUnknownLanguagesAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.unknown")
	if err := os.WriteFile(path, []byte(bigSourceFile()), 0o644); err != nil {
		t.Fatal(err)
	}

	plain, err := generateFingerprint(path, dir, options{keepHeaders: true})
	if err != nil {
		t.Fatal(err)
	}
	stripped, err := generateFingerprint(path, dir, options{})
	if err != nil {
		t.Fatal(err)
	}

	if plain.Fingerprint != stripped.Fingerprint {
		t.Error("an unmapped extension should be fingerprinted whole")
	}
}

// The option reaches the workers, not just generateFingerprint.
func TestWithSkipHeadersReachesTheWorkers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.c")
	if err := os.WriteFile(path, []byte(bigSourceFile()), 0o644); err != nil {
		t.Fatal(err)
	}

	res := Files([]string{path}, 1, dir, nil, WithSkipHeaders(0))
	if len(res.Errors) > 0 {
		t.Fatalf("errors: %v", res.Errors)
	}
	if !strings.Contains(string(res.WFP), "start_line=") {
		t.Error("Files did not apply WithSkipHeaders")
	}

	var buf bytes.Buffer
	if _, err := Stream([]string{path}, 1, dir, &buf, nil, WithSkipHeaders(0)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "start_line=") {
		t.Error("Stream did not apply WithSkipHeaders")
	}
}

// entryPointWFPs fingerprints one file through every public entry point with the same options,
// so a test can hold each of them to the same answer.
func entryPointWFPs(t *testing.T, dir, path string, opts ...Option) map[string]string {
	t.Helper()
	out := map[string]string{}

	res := Files([]string{path}, 1, dir, nil, opts...)
	if len(res.Errors) > 0 {
		t.Fatalf("Files: %v", res.Errors)
	}
	out["Files"] = string(res.WFP)

	var buf bytes.Buffer
	if _, err := Stream([]string{path}, 1, dir, &buf, nil, opts...); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	out["Stream"] = buf.String()

	folder, err := Folder(dir, nil, 1, nil, opts...)
	if err != nil {
		t.Fatalf("Folder: %v", err)
	}
	out["Folder"] = string(folder.WFP)

	buf.Reset()
	if _, err := StreamFolder(dir, nil, 1, &buf, nil, opts...); err != nil {
		t.Fatalf("StreamFolder: %v", err)
	}
	out["StreamFolder"] = buf.String()
	return out
}

func writeBigSource(t *testing.T) (dir, path string) {
	t.Helper()
	dir = t.TempDir()
	path = filepath.Join(dir, "big.c")
	if err := os.WriteFile(path, []byte(bigSourceFile()), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, path
}

// A caller passing no option gets the header filter, with no limit, from every entry point: the
// library default matches the CLI's.
func TestHeaderFilterIsOnByDefault(t *testing.T) {
	dir, path := writeBigSource(t)
	offset := headerOffset(path, bigSourceFile(), 0)
	if offset <= 1 {
		t.Fatalf("fixture should have a multi-line header to strip, got offset %d", offset)
	}
	marker := fmt.Sprintf("start_line=%d\n", offset)

	for entry, got := range entryPointWFPs(t, dir, path) {
		if !strings.Contains(got, marker) {
			t.Errorf("%s with no options: missing %q; got:\n%s", entry, marker, got)
		}
	}
}

// WithoutSkipHeaders fingerprints the file whole, from every entry point.
func TestWithoutSkipHeadersFingerprintsWhole(t *testing.T) {
	dir, path := writeBigSource(t)
	whole, err := generateFingerprint(path, dir, options{keepHeaders: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(whole.Fingerprint, "start_line=") {
		t.Fatal("the unfiltered reference must not carry a start_line marker")
	}

	for entry, got := range entryPointWFPs(t, dir, path, WithoutSkipHeaders()) {
		if strings.Contains(got, "start_line=") {
			t.Errorf("%s with WithoutSkipHeaders: WFP carries a start_line marker:\n%s", entry, got)
		}
		if !strings.Contains(got, whole.Fingerprint) {
			t.Errorf("%s with WithoutSkipHeaders: WFP differs from the whole-file fingerprint", entry)
		}
	}
}

// WithSkipHeaders(limit) still caps the filter, and of it and WithoutSkipHeaders the last wins.
func TestWithSkipHeadersHonoursTheLimit(t *testing.T) {
	dir, path := writeBigSource(t)
	if offset := headerOffset(path, bigSourceFile(), 0); offset <= 2 {
		t.Fatalf("fixture header must be longer than the limit, got offset %d", offset)
	}

	for entry, got := range entryPointWFPs(t, dir, path, WithSkipHeaders(2)) {
		if !strings.Contains(got, "start_line=2\n") {
			t.Errorf("%s with WithSkipHeaders(2): want start_line=2; got:\n%s", entry, got)
		}
	}

	res := Files([]string{path}, 1, dir, nil, WithoutSkipHeaders(), WithSkipHeaders(2))
	if !strings.Contains(string(res.WFP), "start_line=2\n") {
		t.Errorf("WithSkipHeaders after WithoutSkipHeaders should switch the filter back on; got:\n%s", res.WFP)
	}
	res = Files([]string{path}, 1, dir, nil, WithSkipHeaders(2), WithoutSkipHeaders())
	if strings.Contains(string(res.WFP), "start_line=") {
		t.Errorf("WithoutSkipHeaders after WithSkipHeaders should switch the filter off; got:\n%s", res.WFP)
	}
}
