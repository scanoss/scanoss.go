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

	spdxjson "github.com/spdx/tools-golang/json"
	"github.com/spdx/tools-golang/spdxlib"
)

func TestSPDXLite_Structure(t *testing.T) {
	out, err := Generate(sampleInventory(), FormatSPDX, WithProjectName("my-project"))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	doc, err := spdxjson.Read(strings.NewReader(out))
	if err != nil {
		t.Fatalf("read SPDX: %v", err)
	}

	if doc.SPDXVersion != "SPDX-2.3" {
		t.Errorf("spdxVersion = %q", doc.SPDXVersion)
	}
	if doc.DataLicense != "CC0-1.0" {
		t.Errorf("dataLicense = %q", doc.DataLicense)
	}
	if doc.DocumentName != "SBOM for my-project" {
		t.Errorf("name = %q", doc.DocumentName)
	}
	if !strings.HasPrefix(doc.DocumentNamespace, "https://spdx.org/spdxdocs/my-project-") {
		t.Errorf("documentNamespace = %q", doc.DocumentNamespace)
	}
	if len(doc.Packages) != 2 {
		t.Fatalf("want 2 packages, got %d", len(doc.Packages))
	}

	// SPDX requires a DESCRIBES relationship per package (multi-package docs).
	if len(doc.Relationships) != len(doc.Packages) {
		t.Errorf("want %d DESCRIBES relationships, got %d", len(doc.Packages), len(doc.Relationships))
	}
	for _, r := range doc.Relationships {
		if r.Relationship != "DESCRIBES" || string(r.RefA.ElementRefID) != "DOCUMENT" {
			t.Errorf("unexpected relationship: %+v", r)
		}
	}

	// Official SPDX validation (relationship IDs must resolve to real elements).
	if err := spdxlib.ValidateDocument(doc); err != nil {
		t.Errorf("spdxlib.ValidateDocument: %v", err)
	}

	var hasOrg bool
	for _, c := range doc.CreationInfo.Creators {
		if c.CreatorType == "Organization" && c.Creator == "SCANOSS" {
			hasOrg = true
		}
	}
	if !hasOrg {
		t.Errorf("creators missing Organization SCANOSS: %+v", doc.CreationInfo.Creators)
	}
}

func TestSPDXLite_MultiplePurlExternalRefs(t *testing.T) {
	inv := Inventory{Components: []Component{{
		Purl:       "pkg:github/scanoss/scanoss.js",
		AliasPurls: []string{"pkg:npm/scanoss"},
		Version:    "1.3.0",
	}}}

	out, err := Generate(inv, FormatSPDX)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	doc, err := spdxjson.Read(strings.NewReader(out))
	if err != nil {
		t.Fatalf("read SPDX: %v", err)
	}

	refs := doc.Packages[0].PackageExternalReferences
	var purls []string
	for _, r := range refs {
		if r.RefType == "purl" {
			purls = append(purls, r.Locator)
		}
	}
	if len(purls) != 2 || purls[0] != "pkg:github/scanoss/scanoss.js" || purls[1] != "pkg:npm/scanoss" {
		t.Errorf("purl externalRefs = %v, want both purls", purls)
	}
}

func TestSPDXLite_Package(t *testing.T) {
	out, err := Generate(sampleInventory(), FormatSPDX)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	doc, err := spdxjson.Read(strings.NewReader(out))
	if err != nil {
		t.Fatalf("read SPDX: %v", err)
	}

	pkg := doc.Packages[0]
	if pkg.PackageName != "pkg:github/scanoss/engine" {
		t.Errorf("name = %q", pkg.PackageName)
	}
	if pkg.PackageVersion != "5.4.1" {
		t.Errorf("versionInfo = %q", pkg.PackageVersion)
	}
	if pkg.PackageSupplier == nil || pkg.PackageSupplier.Supplier != "scanoss" {
		t.Errorf("supplier = %+v", pkg.PackageSupplier)
	}
	if pkg.PackageLicenseDeclared != "MIT AND Apache-2.0" {
		t.Errorf("licenseDeclared = %q", pkg.PackageLicenseDeclared)
	}
	if pkg.PackageLicenseConcluded != "NOASSERTION" {
		t.Errorf("licenseConcluded = %q, want NOASSERTION (no concluded licenses)", pkg.PackageLicenseConcluded)
	}
	var purlRef, hashRef string
	for _, r := range pkg.PackageExternalReferences {
		switch r.RefType {
		case "purl":
			purlRef = r.Locator
		case "scanoss-url-hash":
			hashRef = r.Locator
		}
	}
	if purlRef != "pkg:github/scanoss/engine" {
		t.Errorf("purl externalRef = %q", purlRef)
	}
	// url_hash (a CRC64) is preserved as an OTHER externalRef, not an (invalid) MD5 checksum.
	if hashRef != "abc123" {
		t.Errorf("scanoss-url-hash externalRef = %q, want abc123", hashRef)
	}
	if len(pkg.PackageChecksums) != 0 {
		t.Errorf("expected no checksums, got %+v", pkg.PackageChecksums)
	}
}

func TestSPDXLite_ExtractedLicenses(t *testing.T) {
	out, err := Generate(sampleInventory(), FormatSPDX)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	doc, err := spdxjson.Read(strings.NewReader(out))
	if err != nil {
		t.Fatalf("read SPDX: %v", err)
	}
	if len(doc.OtherLicenses) != 1 {
		t.Fatalf("want 1 extracted license, got %d", len(doc.OtherLicenses))
	}
	if doc.OtherLicenses[0].LicenseIdentifier != "LicenseRef-scancode-unknown" {
		t.Errorf("extracted licenseId = %q", doc.OtherLicenses[0].LicenseIdentifier)
	}
}

func TestSPDXLite_LicenseConcluded(t *testing.T) {
	inv := Inventory{Components: []Component{{
		Purl:    "pkg:github/test/proj",
		Version: "1.0.0",
		Licenses: []License{
			{ID: "GPL-2.0-only", Acknowledgement: AckDeclared},
			{ID: "3D-Slicer-1.0", Acknowledgement: AckConcluded},
		},
	}}}
	out, err := Generate(inv, FormatSPDX)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	doc, err := spdxjson.Read(strings.NewReader(out))
	if err != nil {
		t.Fatalf("read SPDX: %v", err)
	}
	pkg := doc.Packages[0]
	if pkg.PackageLicenseDeclared != "GPL-2.0-only" {
		t.Errorf("licenseDeclared = %q, want GPL-2.0-only", pkg.PackageLicenseDeclared)
	}
	if pkg.PackageLicenseConcluded != "3D-Slicer-1.0" {
		t.Errorf("licenseConcluded = %q, want 3D-Slicer-1.0", pkg.PackageLicenseConcluded)
	}
}
