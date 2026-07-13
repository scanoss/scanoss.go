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
