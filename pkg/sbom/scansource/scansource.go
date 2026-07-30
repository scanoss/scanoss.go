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

// Package scansource adapts SCANOSS SDK values — a v3 scan result and the licenses,
// vulnerabilities, cryptography and geoprovenance decoration responses — into the
// neutral sbom.Inventory consumed by the sbom package. It is the only SBOM code that
// depends on the scan SDK; the sbom package itself stays SDK-free.
package scansource

import (
	"sort"
	"strings"

	scanossapi "github.com/scanoss/scanoss.api-sdk"
	"github.com/scanoss/scanoss.go/pkg/sbom"
)

// FromScanResult builds an Inventory from a v3 scan result. The deduplicated component
// catalog becomes the components; each component's matched files (joined by url_hash)
// become its file evidence. The version is taken from the component entry. Licenses and
// vulnerabilities are not populated here — they come from decoration services
// (LicensesFrom, VulnerabilitiesFrom).
func FromScanResult(result *scanossapi.ScanResult) sbom.Inventory {
	if result == nil {
		return sbom.Inventory{}
	}

	filesByHash := filesByURLHash(result.Files)

	// Iterate the catalog in sorted url_hash order so output is deterministic
	// (Go map iteration order is random).
	hashes := make([]string, 0, len(result.Components))
	for hash := range result.Components {
		hashes = append(hashes, hash)
	}
	sort.Strings(hashes)

	components := make([]sbom.Component, 0, len(hashes))
	for _, hash := range hashes {
		comp := result.Components[hash]
		if len(comp.Purls) == 0 || comp.Purls[0] == "" {
			continue
		}
		components = append(components, sbom.Component{
			Purl:       comp.Purls[0],
			Scope:      sbom.ScopeDetected,
			AliasPurls: comp.Purls[1:],
			Vendor:     comp.Vendor,
			Name:       comp.Component,
			Version:    comp.Version,
			URL:        comp.Url,
			URLHash:    hash,
			Evidence:   filesByHash[hash],
		})
	}
	return sbom.Inventory{Components: components}
}

// LicenseKey is the join key matching a decoration response entry to a component: its PURL plus
// the queried version (the decoration echoes the queried version back as `requirement`).
func LicenseKey(purl, version string) string {
	return purl + "\x00" + version
}

// LicensesFrom maps a licenses decoration response into declared licenses keyed by
// LicenseKey(purl, requirement). Duplicate ids per key are dropped. (The decoration service has
// no declared/concluded distinction — its licenses are declared.)
func LicensesFrom(resp *scanossapi.ComponentsLicenseResponse) map[string][]sbom.License {
	if resp == nil || resp.Components == nil {
		return nil
	}

	byKey := make(map[string][]sbom.License)
	seen := make(map[string]map[string]bool)
	for _, ci := range *resp.Components {
		if ci.Purl == nil || ci.Licenses == nil {
			continue
		}
		key := LicenseKey(*ci.Purl, strVal(ci.Requirement))
		if seen[key] == nil {
			seen[key] = make(map[string]bool)
		}
		for _, l := range *ci.Licenses {
			id := strVal(l.Id)
			if id == "" || seen[key][id] {
				continue
			}
			seen[key][id] = true
			byKey[key] = append(byKey[key], sbom.License{ID: id, Acknowledgement: sbom.AckDeclared})
		}
	}
	return byKey
}

// CryptographyFrom maps a cryptography-algorithms decoration response into algorithms keyed by
// LicenseKey(purl, requirement).
func CryptographyFrom(resp *scanossapi.CryptoAlgorithmsResponse) map[string][]sbom.CryptoAlgorithm {
	if resp == nil {
		return nil
	}
	byKey := make(map[string][]sbom.CryptoAlgorithm)
	for _, ci := range resp.Components {
		if ci.Purl == nil || ci.Algorithms == nil {
			continue
		}
		key := LicenseKey(*ci.Purl, strVal(ci.Requirement))
		for _, a := range *ci.Algorithms {
			name := strVal(a.Algorithm)
			if name == "" {
				continue
			}
			byKey[key] = append(byKey[key], sbom.CryptoAlgorithm{Algorithm: name, Strength: strVal(a.Strength)})
		}
	}
	return byKey
}

// GeoprovenanceFrom maps a geoprovenance-origin decoration response into contributor locations
// keyed by PURL (the response carries no requirement to join on).
func GeoprovenanceFrom(resp *scanossapi.GeoOriginResponse) map[string][]sbom.GeoLocation {
	if resp == nil {
		return nil
	}
	byPurl := make(map[string][]sbom.GeoLocation)
	for _, cl := range resp.ComponentsLocations {
		if cl.Purl == nil || cl.Locations == nil {
			continue
		}
		for _, loc := range *cl.Locations {
			name := strVal(loc.Name)
			if name == "" {
				continue
			}
			g := sbom.GeoLocation{Name: name}
			if loc.Percentage != nil {
				g.Percentage = float64(*loc.Percentage)
			}
			byPurl[*cl.Purl] = append(byPurl[*cl.Purl], g)
		}
	}
	return byPurl
}

// VulnerabilitiesFrom maps a vulnerabilities decoration response into neutral
// vulnerabilities, deduplicated by id (falling back to the CVE), accumulating the
// affected component PURLs.
func VulnerabilitiesFrom(resp *scanossapi.VulnerabilitiesResponse) []sbom.Vulnerability {
	if resp == nil {
		return nil
	}

	byID := make(map[string]*sbom.Vulnerability)
	var order []string

	for _, ci := range resp.Components {
		if ci.Purl == nil || ci.Vulnerabilities == nil {
			continue
		}
		// The version this entry answers for, not the bare PURL. The service reports per version —
		// asked about two releases of the same component it returned 43 advisories for one and 14
		// for the other — so dropping it and keying on the PURL alone imputes every advisory to
		// every version of that component.
		purl := *ci.Purl
		if version := strVal(ci.Version); version != "" {
			purl += "@" + version
		} else if req := strVal(ci.Requirement); req != "" {
			purl += "@" + req
		}
		for _, v := range *ci.Vulnerabilities {
			id := strVal(v.Id)
			if id == "" {
				id = strVal(v.Cve)
			}
			if id == "" {
				continue
			}

			if existing, ok := byID[id]; ok {
				if !containsString(existing.Purls, purl) {
					existing.Purls = append(existing.Purls, purl)
				}
				continue
			}

			vuln := &sbom.Vulnerability{
				ID:       id,
				Severity: strings.ToLower(strVal(v.Severity)),
				URL:      strVal(v.Url),
				Summary:  strVal(v.Summary),
				Purls:    []string{purl},
			}
			if v.Source != nil {
				vuln.Source = string(*v.Source)
			}
			byID[id] = vuln
			order = append(order, id)
		}
	}

	out := make([]sbom.Vulnerability, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out
}

// filesByURLHash groups matched files (file/snippet) by component url_hash, emitting one
// evidence per match, sorted by path for deterministic output.
func filesByURLHash(files []scanossapi.FileResult) map[string][]sbom.FileEvidence {
	byHash := make(map[string][]sbom.FileEvidence)
	for _, f := range files {
		if f.MatchType == "" || f.MatchType == "none" {
			continue
		}
		for _, m := range f.Matches {
			ev := sbom.FileEvidence{
				Path:            f.Path,
				SourceHash:      f.SourceHash,
				FileHash:        f.FileHash,
				MatchType:       string(f.MatchType),
				MatchPercentage: m.MatchPercentage,
				OssFilePath:     m.OssFilePath,
				InputLineRanges: lineRanges(m.InputLineRanges),
				OssLineRanges:   lineRanges(m.OssLineRanges),
			}
			byHash[m.UrlHash] = append(byHash[m.UrlHash], ev)
		}
	}
	for hash := range byHash {
		evs := byHash[hash]
		sort.Slice(evs, func(i, j int) bool { return evs[i].Path < evs[j].Path })
	}
	return byHash
}

// lineRanges maps the scan result's line ranges onto the inventory's own type, or nil when
// there are none.
func lineRanges(ranges []scanossapi.LineRange) []sbom.LineRange {
	if len(ranges) == 0 {
		return nil
	}
	out := make([]sbom.LineRange, 0, len(ranges))
	for _, r := range ranges {
		out = append(out, sbom.LineRange{StartLine: r.StartLine, EndLine: r.EndLine})
	}
	return out
}

func strVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func containsString(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
