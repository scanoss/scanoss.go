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

// TestCollectKeepManifests verifies the split: Scanning prunes dependency
// manifests (they are useless for fingerprint matching), while Dependencies
// applies the same prune but keeps manifests for the dependency parser. Non-manifest files sharing a manifest extension (data.json) and
// manifests nested inside skipped dirs (node_modules) are NOT kept.
func TestCollectKeepManifests(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.go"), 200)                             // source — kept by both
	writeFile(t, filepath.Join(root, "package.json"), 200)                        // manifest
	writeFile(t, filepath.Join(root, "go.mod"), 200)                              // manifest
	writeFile(t, filepath.Join(root, "sub", "pom.xml"), 200)                      // nested manifest (sub not skipped)
	writeFile(t, filepath.Join(root, "data.json"), 200)                           // .json but NOT a manifest → skip
	writeFile(t, filepath.Join(root, "logo.png"), 200)                            // ext skip
	writeFile(t, filepath.Join(root, "node_modules", "dep", "package.json"), 200) // dir skip → not descended

	scan, err := Collect(root, Scanning(nil))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := baseNames(scan.Files), []string{"main.go"}; !equalStrings(got, want) {
		t.Fatalf("SourceFiles kept = %v, want %v (manifests must be skipped)", got, want)
	}

	ing, err := Collect(root, Dependencies(nil))
	if err != nil {
		t.Fatal(err)
	}
	got := baseNames(ing.Files) // sorted base names
	want := []string{"go.mod", "main.go", "package.json", "pom.xml"}
	if !equalStrings(got, want) {
		t.Fatalf("Dependencies kept = %v, want %v "+
			"(root manifests kept; data.json/logo.png/node_modules pruned)", got, want)
	}
}

// A rule the project wrote in scanoss.json overrules the manifest exemption.
//
// The exemption exists so the built-in extension list (.json, .mod, .xml) does
// not swallow the manifests a dependency scan needs. It is not there to overrule
// someone who said, explicitly, not to look at a given file — before this, a user
// excluding "examples/go.mod" by name was ignored, and the only way through was
// to prune the whole directory.
func TestUserRulesOverrideTheManifestExemption(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), 200)
	writeFile(t, filepath.Join(root, "examples", "go.mod"), 200)

	opts := Dependencies(nil)
	opts.SkipPatterns = []string{"examples/go.mod"}

	res, err := Collect(root, opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Files {
		rel, _ := filepath.Rel(root, f)
		if filepath.ToSlash(rel) == "examples/go.mod" {
			t.Error("a manifest excluded by name in scanoss.json must not be collected")
		}
	}

	// The built-in rules still yield to the exemption: go.mod survives the
	// default extension list, which is what the exemption is for.
	var found bool
	for _, f := range res.Files {
		if filepath.Base(f) == "go.mod" {
			found = true
		}
	}
	if !found {
		t.Error("the root manifest must still be collected")
	}
}
