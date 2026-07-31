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

package scansource

import (
	"testing"

	scanossapi "github.com/scanoss/scanoss.api-sdk"
	"github.com/scanoss/scanoss.go/pkg/sbom"
)

func sampleResult() *scanossapi.ScanResult {
	return &scanossapi.ScanResult{
		Files: []scanossapi.FileResult{
			{
				Path:      "src/a.c",
				MatchType: "file",
				Matches:   []scanossapi.MatchResult{{UrlHash: "h1"}},
			},
			{
				Path:      "src/b.c",
				MatchType: "snippet",
				Matches: []scanossapi.MatchResult{{
					UrlHash:         "h1",
					InputLineRanges: []scanossapi.LineRange{{StartLine: 12, EndLine: 48}},
					OssLineRanges:   []scanossapi.LineRange{{StartLine: 100, EndLine: 136}},
				}},
			},
			{Path: "ignored.c", MatchType: "none", Matches: nil},
		},
		Components: map[string]scanossapi.ComponentResult{
			"h1": {
				Purls:       []string{"pkg:github/scanoss/engine"},
				Vendor:      "scanoss",
				Component:   "engine",
				Version:     "5.4.1",
				Url:         "https://github.com/scanoss/engine",
				Rank:        6,
				ReleaseDate: "2026-03-12",
				File:        "v5.4.1.zip",
			},
			"h2": {
				Purls:   []string{"pkg:npm/lodash"},
				Version: "4.17.21",
			},
			"h3": {Purls: []string{""}}, // skipped: empty purl
		},
	}
}

func TestFromScanResult_Extraction(t *testing.T) {
	inv := FromScanResult(sampleResult())
	if len(inv.Components) != 2 {
		t.Fatalf("want 2 components (empty-purl skipped), got %d", len(inv.Components))
	}

	// Components are sorted by url_hash: h1 (engine) before h2 (lodash).
	engine := inv.Components[0]
	if engine.Purl != "pkg:github/scanoss/engine" || engine.Vendor != "scanoss" {
		t.Errorf("engine mapping wrong: %+v", engine)
	}
	if engine.Version != "5.4.1" {
		t.Errorf("version should come from the component entry, got %q", engine.Version)
	}
	if len(engine.Licenses) != 0 {
		t.Errorf("FromScanResult should not populate licenses (they come from decoration), got %v", engine.Licenses)
	}

	lodash := inv.Components[1]
	if lodash.Version != "4.17.21" {
		t.Errorf("component version should map through, got %q", lodash.Version)
	}
}

// Every field the engine reports about a component reaches the inventory. Rank in
// particular: two components matching the same file are told apart by nothing else.
func TestFromScanResult_ComponentFieldsAreNotDropped(t *testing.T) {
	engine := FromScanResult(sampleResult()).Components[0]

	if engine.Rank != 6 {
		t.Errorf("Rank = %d, want 6", engine.Rank)
	}
	if engine.ReleaseDate != "2026-03-12" {
		t.Errorf("ReleaseDate = %q, want 2026-03-12", engine.ReleaseDate)
	}
	if engine.ArtifactName != "v5.4.1.zip" {
		t.Errorf("ArtifactName = %q, want v5.4.1.zip", engine.ArtifactName)
	}
	if engine.URL != "https://github.com/scanoss/engine" || engine.URLHash != "h1" {
		t.Errorf("URL/URLHash = %q/%q", engine.URL, engine.URLHash)
	}
	if engine.Name != "engine" {
		t.Errorf("Name = %q, want engine", engine.Name)
	}
}

// A component the engine reported without them leaves them empty rather than inventing a
// value — the fields are omitempty, so an absent rank must not render as 0.
func TestFromScanResult_AbsentComponentFieldsStayEmpty(t *testing.T) {
	lodash := FromScanResult(sampleResult()).Components[1]

	if lodash.Rank != 0 || lodash.ReleaseDate != "" || lodash.ArtifactName != "" {
		t.Errorf("want the three unset, got rank=%d release=%q artifact=%q",
			lodash.Rank, lodash.ReleaseDate, lodash.ArtifactName)
	}
}

func TestFromScanResult_FileEvidence(t *testing.T) {
	inv := FromScanResult(sampleResult())
	files := inv.Components[0].Evidence
	if len(files) != 2 {
		t.Fatalf("want 2 file evidences (none-match excluded), got %d", len(files))
	}
	// sorted by path: a.c before b.c
	if files[0].Path != "src/a.c" || files[1].Path != "src/b.c" {
		t.Errorf("files not sorted by path: %v", files)
	}
	if files[1].MatchType != "snippet" || len(files[1].InputLineRanges) != 1 || files[1].InputLineRanges[0] != (sbom.LineRange{StartLine: 12, EndLine: 48}) {
		t.Errorf("snippet evidence wrong: %+v", files[1])
	}
}

func TestFromScanResult_Nil(t *testing.T) {
	if inv := FromScanResult(nil); len(inv.Components) != 0 {
		t.Errorf("nil result should yield empty inventory, got %+v", inv)
	}
}

func TestFromScanResult_MultiMatchFile(t *testing.T) {
	// One snippet file matching two components yields one file evidence under each
	// component's url_hash, with that match's own line ranges.
	result := &scanossapi.ScanResult{
		Files: []scanossapi.FileResult{
			{
				Path:      "src/multi.js",
				MatchType: "snippet",
				Matches: []scanossapi.MatchResult{
					{UrlHash: "h1", InputLineRanges: []scanossapi.LineRange{{StartLine: 12, EndLine: 48}}},
					{UrlHash: "h2", InputLineRanges: []scanossapi.LineRange{{StartLine: 12, EndLine: 40}}},
				},
			},
		},
		Components: map[string]scanossapi.ComponentResult{
			"h1": {Purls: []string{"pkg:github/scanoss/scanner.c", "pkg:npm/scanner"}, Vendor: "scanoss", Component: "scanner.c", Version: "v1.3.0", Url: "https://github.com/scanoss/scanner.c"},
			"h2": {Purls: []string{"pkg:github/example/other-lib"}, Version: "2.0.1"}, // only the purl and version: the optional fields stay empty
		},
	}

	inv := FromScanResult(result)
	if len(inv.Components) != 2 {
		t.Fatalf("want 2 components, got %d", len(inv.Components))
	}
	// the first purl is the canonical identity; the rest become aliases.
	if inv.Components[0].Purl != "pkg:github/scanoss/scanner.c" {
		t.Errorf("canonical Purl = %q", inv.Components[0].Purl)
	}
	if got := inv.Components[0].AliasPurls; len(got) != 1 || got[0] != "pkg:npm/scanner" {
		t.Errorf("AliasPurls = %v, want [pkg:npm/scanner]", got)
	}
	for _, c := range inv.Components {
		if len(c.Evidence) != 1 || c.Evidence[0].Path != "src/multi.js" {
			t.Errorf("component %s should carry the multi-match file as evidence: %+v", c.Purl, c.Evidence)
		}
	}
	// h1 sorts before h2; each gets its own match's ranges.
	if got := inv.Components[0].Evidence[0].InputLineRanges; len(got) != 1 || got[0] != (sbom.LineRange{StartLine: 12, EndLine: 48}) {
		t.Errorf("h1 evidence ranges = %v, want [{12 48}]", got)
	}
	if got := inv.Components[1].Evidence[0].InputLineRanges; len(got) != 1 || got[0] != (sbom.LineRange{StartLine: 12, EndLine: 40}) {
		t.Errorf("h2 evidence ranges = %v, want [{12 40}]", got)
	}
	// The minimal catalog entry — purl and version only — still maps cleanly.
	other := inv.Components[1]
	if other.Purl != "pkg:github/example/other-lib" || other.Version != "2.0.1" || other.URL != "" {
		t.Errorf("minimal component mapped wrong: %+v", other)
	}
}

func ptr[T any](v T) *T { return &v }

func TestLicensesFrom(t *testing.T) {
	resp := &scanossapi.ComponentsLicenseResponse{
		Components: &[]scanossapi.ComponentLicenseInfo{
			{
				// one entry per queried version of the same base PURL
				Purl:        ptr("pkg:npm/lodash"),
				Requirement: ptr("4.17.20"),
				Licenses: &[]scanossapi.LicenseInfo{
					{Id: ptr("MIT")}, {Id: ptr("ISC")}, {Id: ptr("MIT")}, // in-entry dup dropped
				},
			},
			{
				Purl:        ptr("pkg:npm/lodash"),
				Requirement: ptr("4.17.21"),
				Licenses: &[]scanossapi.LicenseInfo{
					{Id: ptr("MIT")}, {Id: ptr("GPL-2.0-only")}, // distinct key (different version)
				},
			},
			{Purl: ptr("pkg:gem/rails")}, // no licenses -> absent from the map
		},
	}

	byKey := LicensesFrom(resp)

	// Keyed by PURL + requirement (queried version), so each version is distinct.
	v20 := byKey[LicenseKey("pkg:npm/lodash", "4.17.20")]
	if len(v20) != 2 || v20[0].ID != "MIT" || v20[0].Acknowledgement != sbom.AckDeclared || v20[1].ID != "ISC" {
		t.Errorf("4.17.20 licenses = %v, want [MIT ISC] declared (in-entry dup dropped)", v20)
	}
	v21 := byKey[LicenseKey("pkg:npm/lodash", "4.17.21")]
	if len(v21) != 2 || v21[0].ID != "MIT" || v21[1].ID != "GPL-2.0-only" {
		t.Errorf("4.17.21 licenses = %v, want [MIT GPL-2.0-only]", v21)
	}
	if _, ok := byKey[LicenseKey("pkg:gem/rails", "")]; ok {
		t.Error("component with no licenses should be absent from the map")
	}
}

func TestLicensesFrom_Nil(t *testing.T) {
	if v := LicensesFrom(nil); v != nil {
		t.Errorf("nil resp should yield nil, got %v", v)
	}
}

func TestVulnerabilitiesFrom(t *testing.T) {
	src := scanossapi.VulnerabilitySource("NVD")
	resp := &scanossapi.VulnerabilitiesResponse{
		Components: []scanossapi.ComponentVulnerabilityInfo{
			{
				Purl: ptr("pkg:npm/lodash"),
				Vulnerabilities: &[]scanossapi.Vulnerability{
					{Id: ptr("CVE-2021-23337"), Severity: ptr("HIGH"), Source: &src, Summary: ptr("cmd injection")},
					{Cve: ptr("CVE-2020-8203")}, // id empty -> falls back to cve
				},
			},
			{
				// same CVE affecting another component -> purls accumulate
				Purl: ptr("pkg:npm/lodash.template"),
				Vulnerabilities: &[]scanossapi.Vulnerability{
					{Id: ptr("CVE-2021-23337")},
				},
			},
		},
	}

	vulns := VulnerabilitiesFrom(resp)
	if len(vulns) != 2 {
		t.Fatalf("want 2 deduped vulnerabilities, got %d: %+v", len(vulns), vulns)
	}

	first := vulns[0]
	if first.ID != "CVE-2021-23337" {
		t.Errorf("id = %q", first.ID)
	}
	if first.Severity != "high" {
		t.Errorf("severity should be lower-cased, got %q", first.Severity)
	}
	if first.Source != "NVD" {
		t.Errorf("source = %q", first.Source)
	}
	if len(first.Purls) != 2 {
		t.Errorf("purls should accumulate across components, got %v", first.Purls)
	}
	if vulns[1].ID != "CVE-2020-8203" {
		t.Errorf("second id should fall back to cve, got %q", vulns[1].ID)
	}
}

func TestVulnerabilitiesFrom_Nil(t *testing.T) {
	if v := VulnerabilitiesFrom(nil); v != nil {
		t.Errorf("nil resp should yield nil, got %v", v)
	}
}

// The service answers per version — asked about two releases of one component it returns different
// advisories for each — so an advisory has to record which version it came back for. Keying on the
// bare PURL attached every advisory to every version, reporting vulnerabilities a release did not
// have.
func TestVulnerabilitiesKeepTheVersionTheyAnswerFor(t *testing.T) {
	old, recent := "v1.24.0", "v2.7.8"
	purl := "pkg:github/denoland/deno"
	cveOld, cveBoth := "CVE-2024-27931", "CVE-2023-0001"

	resp := &scanossapi.VulnerabilitiesResponse{
		Components: []scanossapi.ComponentVulnerabilityInfo{
			{
				Purl: &purl, Version: &old,
				Vulnerabilities: &[]scanossapi.Vulnerability{{Cve: &cveOld}, {Cve: &cveBoth}},
			},
			{
				Purl: &purl, Version: &recent,
				Vulnerabilities: &[]scanossapi.Vulnerability{{Cve: &cveBoth}},
			},
		},
	}

	byID := map[string][]string{}
	for _, v := range VulnerabilitiesFrom(resp) {
		byID[v.ID] = v.Purls
	}

	if got := byID[cveOld]; len(got) != 1 || got[0] != purl+"@"+old {
		t.Errorf("%s affects %v, want only %s@%s — it was not reported for the newer release",
			cveOld, got, purl, old)
	}
	if got := byID[cveBoth]; len(got) != 2 {
		t.Errorf("%s affects %v, want both releases", cveBoth, got)
	}
}

// Without a version the entry still has to land somewhere: the bare PURL, as before.
func TestVulnerabilitiesWithoutAVersionKeepTheBarePurl(t *testing.T) {
	purl, cve := "pkg:npm/lodash", "CVE-2020-0001"
	resp := &scanossapi.VulnerabilitiesResponse{
		Components: []scanossapi.ComponentVulnerabilityInfo{
			{Purl: &purl, Vulnerabilities: &[]scanossapi.Vulnerability{{Cve: &cve}}},
		},
	}
	got := VulnerabilitiesFrom(resp)
	if len(got) != 1 || len(got[0].Purls) != 1 || got[0].Purls[0] != purl {
		t.Errorf("purls = %v, want [%s]", got, purl)
	}
}
