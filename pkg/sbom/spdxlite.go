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
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
	"time"

	spdxjson "github.com/spdx/tools-golang/json"
	"github.com/spdx/tools-golang/spdx/v2/common"
	"github.com/spdx/tools-golang/spdx/v2/v2_3"

	"github.com/scanoss/scanoss.go/internal/config"
)

// spdxTimeFormat is the SPDX creation-timestamp layout (UTC, second precision).
const spdxTimeFormat = "2006-01-02T15:04:05Z"

// scanossURLHashRef is the SPDX OTHER external-reference type carrying the scanoss url_hash
// (a CRC64), which has no standard SPDX checksum algorithm.
const scanossURLHashRef = "scanoss-url-hash"

// buildSPDXLite renders the inventory as an SPDX 2.3 JSON document (restricted to the
// SPDX Lite field subset) via the official tools-golang library. Vulnerabilities and
// file evidence do not apply to SPDX.
func buildSPDXLite(inv Inventory, o options) (string, error) {
	doc := &v2_3.Document{
		SPDXVersion:       v2_3.Version,
		DataLicense:       v2_3.DataLicense,
		SPDXIdentifier:    "DOCUMENT",
		DocumentName:      fmt.Sprintf("SBOM for %s", o.projectName),
		DocumentNamespace: spdxNamespace(o.projectName, inv.Components),
		CreationInfo: &v2_3.CreationInfo{
			Creators: []common.Creator{
				{CreatorType: "Tool", Creator: fmt.Sprintf("%s-%s", config.AppName, config.AppVersion)},
				{CreatorType: "Organization", Creator: config.OrganizationName},
			},
			Created: time.Now().UTC().Format(spdxTimeFormat),
		},
	}

	seenLicenseRefs := make(map[string]bool)
	for _, comp := range inv.Components {
		pkg := spdxPackage(comp)
		doc.Packages = append(doc.Packages, pkg)
		// Each package is described by the document. SPDX requires at least one
		// DESCRIBES relationship when a document has multiple packages.
		doc.Relationships = append(doc.Relationships, &v2_3.Relationship{
			RefA:         common.MakeDocElementID("", "DOCUMENT"),
			RefB:         common.MakeDocElementID("", string(pkg.PackageSPDXIdentifier)),
			Relationship: common.TypeRelationshipDescribe,
		})
		doc.OtherLicenses = append(doc.OtherLicenses, extractedLicenses(comp.Licenses, seenLicenseRefs)...)
	}

	var buf bytes.Buffer
	if err := spdxjson.Write(doc, &buf, spdxjson.Indent("  ")); err != nil {
		return "", fmt.Errorf("error serializing SPDX document: %w", err)
	}
	return buf.String(), nil
}

func spdxPackage(comp Component) *v2_3.Package {
	versionInfo := comp.Version
	if versionInfo == "" {
		versionInfo = noAssertion
	}

	downloadLocation := comp.DownloadLocation
	if downloadLocation == "" {
		downloadLocation = comp.URL
	}
	if downloadLocation == "" {
		downloadLocation = noAssertion
	}

	homepage := comp.URL
	if homepage == "" {
		homepage = noAssertion
	}

	pkg := &v2_3.Package{
		PackageName:               comp.Purl,
		PackageSPDXIdentifier:     common.ElementID(md5Hash(comp.Purl + "@" + comp.Version)),
		PackageVersion:            versionInfo,
		PackageSupplier:           spdxSupplier(comp),
		PackageDownloadLocation:   downloadLocation,
		FilesAnalyzed:             false,
		PackageHomePage:           homepage,
		PackageLicenseDeclared:    joinLicenseIDs(comp.Licenses, AckDeclared),
		PackageLicenseConcluded:   joinLicenseIDs(comp.Licenses, AckConcluded),
		PackageCopyrightText:      noAssertion,
		PackageExternalReferences: purlExternalRefs(comp),
	}

	// The scanoss url_hash is a CRC64, which SPDX 2.3 has no checksum algorithm for, so it
	// is preserved as an OTHER external reference rather than emitted as an (invalid) MD5
	// checksum.
	if comp.URLHash != "" {
		pkg.PackageExternalReferences = append(pkg.PackageExternalReferences, &v2_3.PackageExternalReference{
			Category: "OTHER",
			RefType:  scanossURLHashRef,
			Locator:  comp.URLHash,
		})
	}

	return pkg
}

// purlExternalRefs emits one PACKAGE-MANAGER purl reference per component PURL (SPDX
// supports multiple), so secondary PURLs are not lost.
func purlExternalRefs(comp Component) []*v2_3.PackageExternalReference {
	purls := comp.AllPurls()
	refs := make([]*v2_3.PackageExternalReference, 0, len(purls))
	for _, p := range purls {
		refs = append(refs, &v2_3.PackageExternalReference{Category: "PACKAGE-MANAGER", RefType: "purl", Locator: p})
	}
	return refs
}

func spdxSupplier(comp Component) *common.Supplier {
	if s := supplier(comp); s != noAssertion {
		return &common.Supplier{Supplier: s, SupplierType: "Organization"}
	}
	return &common.Supplier{Supplier: noAssertion}
}

// spdxNamespace builds a deterministic document namespace from the project name and the
// component set (a stable SHA-256), avoiding any time/random nondeterminism.
func spdxNamespace(projectName string, comps []Component) string {
	var b strings.Builder
	b.WriteString(projectName)
	for _, c := range comps {
		b.WriteString("\n")
		b.WriteString(c.Purl)
		b.WriteString("@")
		b.WriteString(c.Version)
	}
	clean := strings.ReplaceAll(projectName, " ", "")
	return fmt.Sprintf("https://spdx.org/spdxdocs/%s-%x", clean, sha256.Sum256([]byte(b.String())))
}

// joinLicenseIDs returns the component's license IDs for the given acknowledgement
// (an empty acknowledgement counts as declared), de-duplicated and joined with " AND ",
// or NOASSERTION when there are none.
func joinLicenseIDs(licenses []License, ack LicenseAcknowledgement) string {
	seen := make(map[string]bool)
	var ids []string
	for _, l := range licenses {
		if l.ID == "" || effectiveAck(l.Acknowledgement) != ack || seen[l.ID] {
			continue
		}
		seen[l.ID] = true
		ids = append(ids, l.ID)
	}
	if len(ids) == 0 {
		return noAssertion
	}
	return strings.Join(ids, " AND ")
}

// effectiveAck treats an unset acknowledgement as declared.
func effectiveAck(ack LicenseAcknowledgement) LicenseAcknowledgement {
	if ack == AckConcluded {
		return AckConcluded
	}
	return AckDeclared
}

// licenseRefPattern matches LicenseRef-* identifiers.
var licenseRefPattern = regexp.MustCompile(`^LicenseRef-(scancode-|scanoss-|)(\S+)$`)

// extractedLicenses maps a component's LicenseRef-* licenses to SPDX
// hasExtractedLicensingInfos, skipping ids already recorded in seen.
func extractedLicenses(licenses []License, seen map[string]bool) []*v2_3.OtherLicense {
	var infos []*v2_3.OtherLicense
	for _, l := range licenses {
		matches := licenseRefPattern.FindStringSubmatch(l.ID)
		if matches == nil || seen[l.ID] {
			continue
		}
		seen[l.ID] = true

		source := strings.TrimSuffix(matches[1], "-")
		sourceText := "."
		if source != "" {
			sourceText = " by " + source + "."
		}
		infos = append(infos, &v2_3.OtherLicense{
			LicenseIdentifier: l.ID,
			LicenseName:       strings.ReplaceAll(matches[2], "-", " "),
			ExtractedText:     "Detected license, please review component source code.",
			LicenseComment:    "Detected license" + sourceText,
		})
	}
	return infos
}

func md5Hash(s string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(s)))
}
