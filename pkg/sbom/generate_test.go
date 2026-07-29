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

func TestGenerate_Dispatch(t *testing.T) {
	inv := sampleInventory()

	cyclone, err := Generate(inv, FormatCycloneDX)
	if err != nil {
		t.Fatalf("cyclonedx: %v", err)
	}
	if !strings.Contains(cyclone, `"bomFormat": "CycloneDX"`) {
		t.Error("cyclonedx output does not look like a CycloneDX BOM")
	}

	spdx, err := Generate(inv, FormatSPDX)
	if err != nil {
		t.Fatalf("spdxlite: %v", err)
	}
	if !strings.Contains(spdx, `"spdxVersion": "SPDX-2.3"`) {
		t.Error("spdxlite output does not look like an SPDX document")
	}
}

func TestGenerate_UnknownFormat(t *testing.T) {
	if _, err := Generate(sampleInventory(), Format("xml")); err == nil {
		t.Error("expected error for unknown format")
	}
}

// Every CycloneDX BOM carries a serial number, and the tool's version goes in its
// own field — CycloneDX has one. SPDX does not, so there the two stay joined as
// "name-version", which is that format's convention for the Tool creator.
func TestToolIdentityPerFormat(t *testing.T) {
	inv := Inventory{Components: []Component{{Purl: "pkg:npm/x", Name: "x", Version: "1"}}}

	cdxDoc, err := Generate(inv, FormatCycloneDX, WithTool("scanoss"), WithToolVersion("9.9.9"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cdxDoc, `"serialNumber": "urn:uuid:`) {
		t.Error("CycloneDX must carry a serial number")
	}
	if !strings.Contains(cdxDoc, `"name": "scanoss"`) || !strings.Contains(cdxDoc, `"version": "9.9.9"`) {
		t.Errorf("tool name and version must be separate fields:\n%s", cdxDoc)
	}
	if strings.Contains(cdxDoc, `"scanoss-9.9.9"`) {
		t.Error("the version must not be baked into the tool name")
	}

	spdxDoc, err := Generate(inv, FormatSPDX, WithTool("scanoss"), WithToolVersion("9.9.9"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(spdxDoc, "Tool: scanoss-9.9.9") {
		t.Errorf("SPDX joins them by convention, it has no version field:\n%s", spdxDoc)
	}
}

// Two renders of the same inventory get different serial numbers: the field
// identifies the document, not its content.
func TestSerialNumberIsPerDocument(t *testing.T) {
	inv := Inventory{Components: []Component{{Purl: "pkg:npm/x", Name: "x"}}}
	a, _ := Generate(inv, FormatCycloneDX)
	b, _ := Generate(inv, FormatCycloneDX)
	if serialOf(a) == serialOf(b) {
		t.Error("each rendered document needs its own serial number")
	}
}

func serialOf(doc string) string {
	const key = `"serialNumber": "`
	i := strings.Index(doc, key)
	if i < 0 {
		return ""
	}
	rest := doc[i+len(key):]
	return rest[:strings.Index(rest, `"`)]
}
