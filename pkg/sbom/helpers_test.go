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

func TestSupplier(t *testing.T) {
	tests := []struct {
		name string
		comp Component
		want string
	}{
		{"vendor wins", Component{Vendor: "acme", Purl: "pkg:github/scanoss/engine"}, "acme"},
		{"namespace from purl", Component{Purl: "pkg:github/scanoss/engine"}, "scanoss"},
		{"no vendor no namespace", Component{Purl: "pkg:gem/rails"}, noAssertion},
	}
	for _, tt := range tests {
		if got := supplier(tt.comp); got != tt.want {
			t.Errorf("%s: supplier = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestMapSeverity(t *testing.T) {
	tests := map[string]string{
		"CRITICAL": "critical",
		"High":     "high",
		"moderate": "medium",
		"low":      "low",
		"none":     "none",
		"":         "",
		"weird":    "unknown",
	}
	for in, want := range tests {
		if got := string(mapSeverity(in)); got != want {
			t.Errorf("mapSeverity(%q) = %q, want %q", in, got, want)
		}
	}
}
