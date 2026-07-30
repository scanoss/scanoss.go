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

import (
	"strings"
	"testing"
)

// Every case here came from a real scan: the API is not obliged to send canonical
// SPDX, and both formats validate what we put in their license fields.
func TestNormalizeLicense(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
		ok             bool
	}{
		{"canonical id", "MIT", "MIT", true},
		{"wrong case is corrected", "Zlib-acknowledgement", "zlib-acknowledgement", true},
		{"valid expression", "MIT OR Apache-2.0", "MIT OR Apache-2.0", true},
		{"exception after WITH", "GPL-2.0-only WITH Classpath-exception-2.0", "GPL-2.0-only WITH Classpath-exception-2.0", true},
		{"license ref", "LicenseRef-scancode-unicode", "LicenseRef-scancode-unicode", true},

		// The one that made a real document invalid: an exception used as an
		// operand of AND, which only WITH may take.
		{"exception outside WITH", "GPL-3.0-or-later AND Autoconf-exception-generic-3.0", "", false},
		{"unknown identifier", "totally-made-up", "", false},
		{"empty", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := normalizeLicense(tc.in)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v (got %q)", ok, tc.ok, got)
			}
			if got != tc.want {
				t.Errorf("normalizeLicense(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// A ref nested inside an expression still has to be declared — missing those is
// what produced "LicenseRef used but not defined" on a real scan.
func TestLicenseRefsIn(t *testing.T) {
	for _, tc := range []struct {
		name, in string
		want     []string
	}{
		{"bare ref", "LicenseRef-scancode-unicode", []string{"LicenseRef-scancode-unicode"}},
		{"nested in an expression", "MIT AND LicenseRef-scancode-pcre", []string{"LicenseRef-scancode-pcre"}},
		{"nested in a malformed expression", "GPL-3.0-or-later AND Autoconf-exception-generic-3.0 AND LicenseRef-scancode-public-domain-disclaimer", []string{"LicenseRef-scancode-public-domain-disclaimer"}},
		{"none", "MIT OR Apache-2.0", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := licenseRefsIn(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("licenseRefsIn(%q) = %v, want %v", tc.in, got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("= %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestLicenseRef(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"LicenseRef-already", "LicenseRef-already"},
		{"Some Weird License 2.0", "LicenseRef-Some-Weird-License-2.0"},
		{"has/slashes+plus", "LicenseRef-has-slashes-plus"},
		{"   ", ""},
	} {
		if got := licenseRef(tc.in); got != tc.want {
			t.Errorf("licenseRef(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// An unrecognised identifier must not reach licenseDeclared bare, and the ref
// that replaces it must be declared.
func TestSPDXUnrecognisedLicenseBecomesDeclaredRef(t *testing.T) {
	inv := Inventory{Components: []Component{{
		Purl: "pkg:npm/x", Name: "x", Version: "1",
		Licenses: []License{{ID: "GPL-3.0-or-later AND Autoconf-exception-generic-3.0"}},
	}}}

	doc, err := Generate(inv, FormatSPDX)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(doc, "Autoconf-exception-generic-3.0 AND") ||
		strings.Contains(doc, "AND Autoconf-exception-generic-3.0") {
		t.Errorf("the malformed expression must not reach licenseDeclared:\n%s", doc)
	}
	if !strings.Contains(doc, "hasExtractedLicensingInfos") {
		t.Errorf("the substitute ref must be declared:\n%s", doc)
	}
}

// Joining expressions needs parentheses: "A OR B" AND "C" is not "A OR (B AND C)".
func TestSPDXParenthesisesExpressions(t *testing.T) {
	inv := Inventory{Components: []Component{{
		Purl: "pkg:npm/x", Name: "x",
		Licenses: []License{{ID: "MIT OR Apache-2.0"}, {ID: "ISC"}},
	}}}
	doc, err := Generate(inv, FormatSPDX)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc, "(MIT OR Apache-2.0) AND ISC") {
		t.Errorf("expected the expression parenthesised before joining:\n%s", doc)
	}
}

// CycloneDX validates license.id against the SPDX enum, so only a recognised
// single identifier may go there — an expression or a ref goes to name.
func TestCycloneDXLicenseFieldChoice(t *testing.T) {
	for _, tc := range []struct {
		name, id, wantField, wantValue string
	}{
		{"canonical", "MIT", "id", "MIT"},
		{"normalised", "Zlib-acknowledgement", "id", "zlib-acknowledgement"},
		{"expression goes to name", "MIT OR Apache-2.0", "name", "MIT OR Apache-2.0"},
		{"ref goes to name", "LicenseRef-scancode-unicode", "name", "LicenseRef-scancode-unicode"},
		{"unknown goes to name", "totally-made-up", "name", "totally-made-up"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lic := cycloneDXLicense(License{ID: tc.id})
			got, field := lic.ID, "id"
			if lic.Name != "" {
				got, field = lic.Name, "name"
			}
			if field != tc.wantField || got != tc.wantValue {
				t.Errorf("%q -> %s=%q, want %s=%q", tc.id, field, got, tc.wantField, tc.wantValue)
			}
		})
	}
}

// A field restricted to the SPDX identifier list rejects every expression, WITH included. One
// component licensed "GPL-2.0-only WITH Classpath-exception-2.0" — a common Java combination —
// used to reach CycloneDX's license.id and invalidate the whole document.
func TestExpressionsNeverReachAnIdentifierField(t *testing.T) {
	for _, expr := range []string{
		"GPL-2.0-only WITH Classpath-exception-2.0",
		"Apache-2.0 OR MIT",
		"MIT AND BSD-3-Clause",
		"(MIT OR Apache-2.0) AND GPL-2.0-only WITH Classpath-exception-2.0",
	} {
		if !isExpression(expr) {
			t.Errorf("isExpression(%q) = false: it would be written to a field that only accepts identifiers", expr)
		}
	}
	for _, id := range []string{"MIT", "Apache-2.0", "GPL-2.0-only", "LicenseRef-Custom"} {
		if isExpression(id) {
			t.Errorf("isExpression(%q) = true: a single identifier is not an expression", id)
		}
	}
}

// Parenthesising is a narrower question than being an expression: WITH binds tighter than AND and
// OR, so it needs none.
func TestOnlyAndOrNeedParentheses(t *testing.T) {
	if isCompound("GPL-2.0-only WITH Classpath-exception-2.0") {
		t.Error("WITH binds tighter than AND/OR, so it does not need parentheses when joined")
	}
	for _, expr := range []string{"Apache-2.0 OR MIT", "MIT AND BSD-3-Clause"} {
		if !isCompound(expr) {
			t.Errorf("isCompound(%q) = false, want true", expr)
		}
	}
}
