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

package fingerprint

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "update the WFP golden file")

// TestGenerateFingerprintGolden pins the exact WFP output for a fixed source file.
// It guards refactors of the fingerprint assembly against any byte-level change.
// Regenerate the golden with: go test ./pkg/fingerprint/wfp -run Golden -update
func TestGenerateFingerprintGolden(t *testing.T) {
	fp, err := GenerateFingerprint(filepath.Join("testdata", "sample.c"), "testdata")
	if err != nil {
		t.Fatalf("GenerateFingerprint: %v", err)
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

	fp, err := GenerateFingerprint(path, dir)
	if err != nil {
		t.Fatalf("GenerateFingerprint: %v", err)
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
			fp, err := GenerateFingerprint(path, tc.root)
			if err != nil {
				t.Fatalf("GenerateFingerprint: %v", err)
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

	fp, err := GenerateFingerprint(path, dir)
	if err != nil {
		t.Fatalf("GenerateFingerprint: %v", err)
	}
	if fp.Size != 0 {
		t.Errorf("Size = %d, want 0", fp.Size)
	}
	if lines := strings.Count(fp.Fingerprint, "\n"); lines != 1 {
		t.Errorf("want only the file= line, got %d lines:\n%s", lines, fp.Fingerprint)
	}
}

func TestGenerateFingerprintUnreadableFile(t *testing.T) {
	_, err := GenerateFingerprint(filepath.Join(t.TempDir(), "missing.c"), "")
	if err == nil {
		t.Fatal("GenerateFingerprint succeeded on a missing file, want an error")
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
		if _, err := GenerateFingerprint(path, ""); err != nil {
			b.Fatalf("GenerateFingerprint: %v", err)
		}
	}
}
