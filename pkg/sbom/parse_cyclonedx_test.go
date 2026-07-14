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

const cdxDoc = `{
  "bomFormat": "CycloneDX",
  "specVersion": "1.7",
  "version": 1,
  "components": [
    {
      "bom-ref": "pkg:github/scanoss/engine@5.4.7",
      "type": "library",
      "name": "pkg:github/scanoss/engine",
      "version": "5.4.7",
      "publisher": "scanoss",
      "purl": "pkg:github/scanoss/engine@5.4.7",
      "licenses": [{"license": {"id": "GPL-2.0-only", "acknowledgement": "declared"}}],
      "externalReferences": [{"url": "https://github.com/scanoss/engine", "type": "website"}]
    }
  ],
  "vulnerabilities": [
    {
      "id": "CVE-2023-1234",
      "source": {"name": "NVD"},
      "ratings": [{"severity": "high"}],
      "description": "Example heap overflow.",
      "advisories": [{"url": "https://nvd.nist.gov/vuln/detail/CVE-2023-1234"}],
      "affects": [{"ref": "pkg:github/scanoss/engine@5.4.7"}]
    }
  ]
}`

func TestParseCycloneDX(t *testing.T) {
	inv, err := ParseCycloneDX([]byte(cdxDoc))
	if err != nil {
		t.Fatalf("ParseCycloneDX: %v", err)
	}

	if len(inv.Components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(inv.Components))
	}
	c := inv.Components[0]
	if c.Purl != "pkg:github/scanoss/engine" {
		t.Errorf("purl: got %q", c.Purl)
	}
	if c.Version != "5.4.7" {
		t.Errorf("version: got %q", c.Version)
	}
	if c.Vendor != "scanoss" {
		t.Errorf("vendor: got %q", c.Vendor)
	}
	if c.URL != "https://github.com/scanoss/engine" {
		t.Errorf("url: got %q", c.URL)
	}
	if len(c.Licenses) != 1 || c.Licenses[0].ID != "GPL-2.0-only" || c.Licenses[0].Acknowledgement != AckDeclared {
		t.Errorf("licenses: got %+v", c.Licenses)
	}

	if len(inv.Vulnerabilities) != 1 {
		t.Fatalf("expected 1 vulnerability, got %d", len(inv.Vulnerabilities))
	}
	v := inv.Vulnerabilities[0]
	if v.ID != "CVE-2023-1234" || v.Severity != "high" || v.Source != "NVD" {
		t.Errorf("vuln fields: got %+v", v)
	}
	if v.Summary != "Example heap overflow." {
		t.Errorf("summary: got %q", v.Summary)
	}
	if len(v.Purls) != 1 || v.Purls[0] != "pkg:github/scanoss/engine" {
		t.Errorf("affects purls: got %v", v.Purls)
	}
}

func TestParseCycloneDX_Invalid(t *testing.T) {
	if _, err := ParseCycloneDX([]byte(`{"not": "a bom"`)); err == nil {
		t.Error("expected an error for malformed CycloneDX")
	}
}
