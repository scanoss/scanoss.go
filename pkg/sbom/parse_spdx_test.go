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

const spdxDoc = `{
  "spdxVersion": "SPDX-2.3",
  "dataLicense": "CC0-1.0",
  "SPDXID": "SPDXRef-DOCUMENT",
  "name": "SBOM for test",
  "documentNamespace": "https://spdx.org/spdxdocs/test-abc",
  "creationInfo": {
    "created": "2026-07-14T00:00:00Z",
    "creators": ["Tool: scanoss", "Organization: SCANOSS"]
  },
  "packages": [
    {
      "SPDXID": "SPDXRef-abc123",
      "name": "pkg:github/scanoss/engine",
      "versionInfo": "5.4.7",
      "supplier": "Organization: scanoss",
      "downloadLocation": "https://github.com/scanoss/engine",
      "filesAnalyzed": false,
      "homepage": "https://github.com/scanoss/engine",
      "licenseDeclared": "GPL-2.0-only",
      "licenseConcluded": "NOASSERTION",
      "copyrightText": "NOASSERTION",
      "checksums": [{"algorithm": "MD5", "checksumValue": "abc123"}],
      "externalRefs": [
        {"referenceCategory": "PACKAGE-MANAGER", "referenceType": "purl", "referenceLocator": "pkg:github/scanoss/engine"}
      ]
    }
  ]
}`

func TestParseSPDX(t *testing.T) {
	inv, err := ParseSPDX([]byte(spdxDoc))
	if err != nil {
		t.Fatalf("ParseSPDX: %v", err)
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
	if c.URLHash != "abc123" {
		t.Errorf("urlhash: got %q", c.URLHash)
	}
	if len(c.Licenses) != 1 || c.Licenses[0].ID != "GPL-2.0-only" || c.Licenses[0].Acknowledgement != AckDeclared {
		t.Errorf("licenses: got %+v", c.Licenses)
	}

	// SPDX 2.3 has no vulnerability model.
	if len(inv.Vulnerabilities) != 0 {
		t.Errorf("expected no vulnerabilities, got %d", len(inv.Vulnerabilities))
	}
}

func TestParseSPDX_Invalid(t *testing.T) {
	if _, err := ParseSPDX([]byte(`{"spdxVersion":`)); err == nil {
		t.Error("expected an error for malformed SPDX")
	}
}
