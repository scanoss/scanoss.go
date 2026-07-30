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
	"reflect"
	"testing"
)

func ptrFloat(f float64) *float64 { return &f }

// TestRoundTrip_CycloneDX writes an Inventory to CycloneDX and reads it back, asserting the
// modeled fields survive. CycloneDX carries components, licenses and vulnerabilities.
func TestRoundTrip_CycloneDX(t *testing.T) {
	inv := Inventory{
		Components: []Component{{
			Purl:     "pkg:github/scanoss/engine",
			Vendor:   "scanoss",
			Version:  "5.4.7",
			URL:      "https://github.com/scanoss/engine",
			URLHash:  "08d3df7638b3a9f1",
			Licenses: []License{{ID: "GPL-2.0-only", Acknowledgement: AckDeclared}},
		}},
		Vulnerabilities: []Vulnerability{{
			ID:         "CVE-2023-1234",
			Severity:   "high",
			Source:     "NVD",
			URL:        "https://nvd.nist.gov/vuln/detail/CVE-2023-1234",
			Summary:    "Example heap overflow.",
			Purls:      []string{"pkg:github/scanoss/engine"},
			CVSSScore:  ptrFloat(7.2),
			CVSSVector: "CVSS:3.1/AV:N/AC:L/PR:H/UI:N/S:U/C:H/I:H/A:H",
			CVSSMethod: "CVSSv31",
			CWEs:       []int{77},
			EPSSScore:  ptrFloat(0.224),
		}},
	}

	doc, err := Generate(inv, FormatCycloneDX)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	got, err := ParseCycloneDX([]byte(doc))
	if err != nil {
		t.Fatalf("ParseCycloneDX: %v", err)
	}
	if !reflect.DeepEqual(inv, got) {
		t.Errorf("round-trip mismatch:\n want %+v\n  got %+v", inv, got)
	}
}

// TestRoundTrip_SPDX writes an Inventory to SPDX and reads it back. SPDX carries components
// and licenses (and the url_hash, as an OTHER external reference) but has no vulnerability
// model, so the inventory has
// no vulnerabilities.
func TestRoundTrip_SPDX(t *testing.T) {
	inv := Inventory{
		Components: []Component{{
			Purl:             "pkg:github/scanoss/engine",
			Vendor:           "scanoss",
			Version:          "5.4.7",
			URL:              "https://github.com/scanoss/engine",
			URLHash:          "08d3df7638b3a9f1",
			DownloadLocation: "https://github.com/scanoss/engine",
			Licenses:         []License{{ID: "GPL-2.0-only", Acknowledgement: AckDeclared}},
		}},
	}

	doc, err := Generate(inv, FormatSPDX)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	got, err := ParseSPDX([]byte(doc))
	if err != nil {
		t.Fatalf("ParseSPDX: %v", err)
	}
	if !reflect.DeepEqual(inv, got) {
		t.Errorf("round-trip mismatch:\n want %+v\n  got %+v", inv, got)
	}
}
