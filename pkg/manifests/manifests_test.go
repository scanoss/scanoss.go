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

package manifests

import "testing"

func TestIs(t *testing.T) {
	keep := []string{
		"package.json", "package-lock.json", "yarn.lock",
		"go.mod", "go.sum", "pom.xml", "build.gradle",
		"requirements.txt", "pyproject.toml",
		"Gemfile", "Gemfile.lock", "packages.config",
		"sub/dir/package.json", // nested — matched on base name
		"MyProject.csproj",     // *.csproj glob
	}
	for _, p := range keep {
		if !Is(p) {
			t.Errorf("Is(%q) = false, want true", p)
		}
	}

	drop := []string{
		"data.json",  // .json but not a manifest name
		"config.xml", // .xml but not pom.xml
		"main.go",    // source
		"README.md",  // doc
		"package.jsonx",
		"notcsproj", // no dot → not matched by *.csproj
	}
	for _, p := range drop {
		if Is(p) {
			t.Errorf("Is(%q) = true, want false", p)
		}
	}
}
