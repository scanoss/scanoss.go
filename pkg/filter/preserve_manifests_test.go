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
	"path/filepath"
	"testing"
)

// TestCollectPreserveDependencyManifests verifies the split: ScanOptions prunes
// dependency manifests (they are useless for fingerprint matching), while
// DependencyOptions applies the same prune but keeps manifests for the dependency
// parser. Non-manifest files sharing a manifest extension (data.json) and
// manifests nested inside skipped dirs (node_modules) are NOT kept.
func TestCollectPreserveDependencyManifests(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.go"), 200)                             // source — kept by both
	writeFile(t, filepath.Join(root, "package.json"), 200)                        // manifest
	writeFile(t, filepath.Join(root, "go.mod"), 200)                              // manifest
	writeFile(t, filepath.Join(root, "sub", "pom.xml"), 200)                      // nested manifest (sub not skipped)
	writeFile(t, filepath.Join(root, "data.json"), 200)                           // .json but NOT a manifest → skip
	writeFile(t, filepath.Join(root, "logo.png"), 200)                            // ext skip
	writeFile(t, filepath.Join(root, "node_modules", "dep", "package.json"), 200) // dir skip → not descended

	scan, err := Collect(root, ScanOptions())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := baseNames(scan.Files), []string{"main.go"}; !equalStrings(got, want) {
		t.Fatalf("ScanOptions kept = %v, want %v (manifests must be skipped)", got, want)
	}

	ing, err := Collect(root, DependencyOptions())
	if err != nil {
		t.Fatal(err)
	}
	got := baseNames(ing.Files) // sorted base names
	want := []string{"go.mod", "main.go", "package.json", "pom.xml"}
	if !equalStrings(got, want) {
		t.Fatalf("DependencyOptions kept = %v, want %v "+
			"(root manifests kept; data.json/logo.png/node_modules pruned)", got, want)
	}
}
