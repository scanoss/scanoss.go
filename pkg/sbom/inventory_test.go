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

package sbom

import "testing"

// The SBOM name field must hold a package name, not a PURL. Detected components
// carry one from the scan result; declared ones (sourced from a manifest) carry
// only a PURL, so the last segment stands in — otherwise every declared package
// in an SBOM would be named "pkg:golang/github.com/spf13/cobra".
func TestComponentDisplayName(t *testing.T) {
	for _, tc := range []struct {
		name string
		comp Component
		want string
	}{
		{"detected keeps its name", Component{Purl: "pkg:github/madler/zlib", Name: "zlib"}, "zlib"},
		{"declared falls back to the purl tail", Component{Purl: "pkg:golang/github.com/spf13/cobra"}, "cobra"},
		{"short purl", Component{Purl: "pkg:npm/lodash"}, "lodash"},
		{"maven namespace", Component{Purl: "pkg:maven/org.apache.commons/commons-lang3"}, "commons-lang3"},
		{"no slash at all", Component{Purl: "weird"}, "weird"},
		{"trailing slash falls back to the whole purl", Component{Purl: "pkg:npm/"}, "pkg:npm/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.comp.DisplayName(); got != tc.want {
				t.Errorf("DisplayName() = %q, want %q", got, tc.want)
			}
		})
	}
}
