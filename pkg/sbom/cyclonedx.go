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
	"fmt"
	"strconv"
	"strings"
	"time"

	cdx "github.com/CycloneDX/cyclonedx-go"

	"github.com/scanoss/scanoss.go/internal/config"
)

// buildCycloneDX renders the inventory as a CycloneDX 1.7 JSON document using the
// official cyclonedx-go encoder, whose version-aware serialization keeps the output
// schema-valid.
func buildCycloneDX(inv Inventory, o options) (string, error) {
	bom := cdx.NewBOM()
	bom.Metadata = &cdx.Metadata{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Authors:   &[]cdx.OrganizationalContact{{Name: config.OrganizationName}},
		Component: &cdx.Component{
			Type:    cdx.ComponentTypeApplication,
			Name:    o.projectName,
			Version: noAssertion,
		},
	}

	comps := make([]cdx.Component, 0, len(inv.Components))
	for _, c := range inv.Components {
		comps = append(comps, cycloneDXComponent(c))
	}
	bom.Components = &comps

	if vulns := cycloneDXVulnerabilities(inv); len(vulns) > 0 {
		bom.Vulnerabilities = &vulns
	}

	var buf bytes.Buffer
	enc := cdx.NewBOMEncoder(&buf, cdx.BOMFileFormatJSON)
	enc.SetPretty(true)
	if err := enc.EncodeVersion(bom, cdx.SpecVersion1_7); err != nil {
		return "", fmt.Errorf("error serializing CycloneDX document: %w", err)
	}
	return buf.String(), nil
}

// cdxPurl returns the component PURL with its version appended when one is known.
func cdxPurl(comp Component) string {
	if comp.Version != "" && comp.Version != noAssertion {
		return comp.Purl + "@" + comp.Version
	}
	return comp.Purl
}

func cycloneDXComponent(comp Component) cdx.Component {
	version := comp.Version
	if version == "" {
		version = noAssertion
	}
	purl := cdxPurl(comp)

	c := cdx.Component{
		BOMRef:     purl,
		Type:       cdx.ComponentTypeLibrary,
		Name:       comp.Purl,
		Version:    version,
		Publisher:  supplier(comp),
		PackageURL: purl,
	}

	if len(comp.Licenses) > 0 {
		var licenses cdx.Licenses
		for _, l := range comp.Licenses {
			lic := cycloneDXLicense(l)
			licenses = append(licenses, cdx.LicenseChoice{License: &lic})
		}
		c.Licenses = &licenses
	}

	if comp.URL != "" {
		c.ExternalReferences = &[]cdx.ExternalReference{
			{Type: cdx.ERTypeWebsite, URL: comp.URL},
		}
	}

	// A CycloneDX component carries a single purl. When a component has more than one,
	// record every purl in evidence.identity (the spec's field for identifying a
	// component by multiple identifiers), so the aliases aren't lost.
	occ := cycloneDXOccurrences(comp.Files)
	ids := purlIdentities(comp)
	if len(occ) > 0 || len(ids) > 0 {
		ev := &cdx.Evidence{}
		if len(occ) > 0 {
			ev.Occurrences = &occ
		}
		if len(ids) > 0 {
			ev.Identity = &cdx.EvidenceIdentityChoice{Identities: &ids}
		}
		c.Evidence = ev
	}

	return c
}

// purlIdentities returns one purl evidence-identity per component PURL (with version
// appended), but only when the component has more than one PURL — a single PURL is
// already the component's canonical purl, so it needs no extra evidence.
func purlIdentities(comp Component) []cdx.EvidenceIdentity {
	purls := comp.AllPurls()
	if len(purls) < 2 {
		return nil
	}
	ids := make([]cdx.EvidenceIdentity, 0, len(purls))
	for _, p := range purls {
		value := p
		if comp.Version != "" && comp.Version != noAssertion {
			value = p + "@" + comp.Version
		}
		ids = append(ids, cdx.EvidenceIdentity{
			Field:          cdx.EvidenceIdentityFieldTypePURL,
			ConcludedValue: value,
		})
	}
	return ids
}

// cycloneDXLicense emits a license with its acknowledgement: a standard SPDX id via
// "id", a LicenseRef-* (or other non-SPDX identifier) via "name".
func cycloneDXLicense(l License) cdx.License {
	lic := cdx.License{Acknowledgement: cdxAcknowledgement(l.Acknowledgement)}
	if strings.HasPrefix(l.ID, "LicenseRef-") {
		lic.Name = l.ID
	} else {
		lic.ID = l.ID
	}
	return lic
}

// cdxAcknowledgement maps a neutral acknowledgement to the CycloneDX enum, defaulting
// to declared.
func cdxAcknowledgement(ack LicenseAcknowledgement) cdx.LicenseAcknowledgement {
	if ack == AckConcluded {
		return cdx.LicenseAcknowledgementConcluded
	}
	return cdx.LicenseAcknowledgementDeclared
}

// cycloneDXOccurrences turns matched files into evidence occurrences. Snippet matches
// carry the start line and the full matched ranges in additionalContext (CycloneDX
// occurrences have no native range field).
func cycloneDXOccurrences(files []FileEvidence) []cdx.EvidenceOccurrence {
	occurrences := make([]cdx.EvidenceOccurrence, 0, len(files))
	for _, f := range files {
		o := cdx.EvidenceOccurrence{Location: f.Path}
		if f.MatchType == "snippet" {
			if start, ok := firstRangeStart(f.InputLineRanges); ok {
				o.Line = &start
			}
			if ctx := rangesContext(f.InputLineRanges, f.OssLineRanges); ctx != "" {
				o.AdditionalContext = ctx
			}
		}
		occurrences = append(occurrences, o)
	}
	return occurrences
}

// firstRangeStart parses the leading line number of the first range (e.g. "12-48").
func firstRangeStart(ranges []string) (int, bool) {
	for _, r := range ranges {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		numStr := r
		if i := strings.IndexAny(r, "-,"); i >= 0 {
			numStr = r[:i]
		}
		if n, err := strconv.Atoi(strings.TrimSpace(numStr)); err == nil {
			return n, true
		}
	}
	return 0, false
}

// rangesContext renders the matched line ranges as human-readable text.
func rangesContext(input, oss []string) string {
	var parts []string
	if len(input) > 0 {
		parts = append(parts, "input lines "+strings.Join(input, ","))
	}
	if len(oss) > 0 {
		parts = append(parts, "oss lines "+strings.Join(oss, ","))
	}
	return strings.Join(parts, "; ")
}

// cycloneDXVulnerabilities emits one BOM-level vulnerability per inventory entry, with
// affects[] resolving to the bom-refs of components whose base PURL it lists.
func cycloneDXVulnerabilities(inv Inventory) []cdx.Vulnerability {
	if len(inv.Vulnerabilities) == 0 {
		return nil
	}

	refsByPurl := make(map[string][]string, len(inv.Components))
	for _, c := range inv.Components {
		refsByPurl[c.Purl] = append(refsByPurl[c.Purl], cdxPurl(c))
	}

	out := make([]cdx.Vulnerability, 0, len(inv.Vulnerabilities))
	for _, v := range inv.Vulnerabilities {
		cv := cdx.Vulnerability{ID: v.ID, Description: v.Summary}
		if v.Source != "" {
			cv.Source = &cdx.Source{Name: v.Source}
		}
		if sev := mapSeverity(v.Severity); sev != "" {
			cv.Ratings = &[]cdx.VulnerabilityRating{{Severity: sev}}
		}
		if v.URL != "" {
			cv.Advisories = &[]cdx.Advisory{{URL: v.URL}}
		}

		var affects []cdx.Affects
		for _, purl := range v.Purls {
			for _, ref := range refsByPurl[purl] {
				affects = append(affects, cdx.Affects{Ref: ref})
			}
		}
		if len(affects) > 0 {
			cv.Affects = &affects
		}

		out = append(out, cv)
	}
	return out
}

// mapSeverity maps a SCANOSS severity string to a CycloneDX severity. An empty input
// yields "" (no rating); an unrecognized value yields "unknown".
func mapSeverity(s string) cdx.Severity {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return ""
	case "critical":
		return cdx.SeverityCritical
	case "high":
		return cdx.SeverityHigh
	case "medium", "moderate":
		return cdx.SeverityMedium
	case "low":
		return cdx.SeverityLow
	case "none":
		return cdx.SeverityNone
	case "info", "informational":
		return cdx.SeverityInfo
	default:
		return cdx.SeverityUnknown
	}
}
