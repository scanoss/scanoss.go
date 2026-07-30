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

package scanpipeline

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	scanossapi "github.com/scanoss/scanoss.api-sdk"

	"github.com/scanoss/scanoss.go/pkg/dependencies/parsers"
	"github.com/scanoss/scanoss.go/pkg/sbom"
	"github.com/scanoss/scanoss.go/pkg/scanoss"
)

// decorationServer answers the licenses, vulnerabilities, dependency-resolve, cryptography,
// geoprovenance and copyright endpoints with canned responses so Build can be exercised
// without a live API.
func decorationServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/dependencies/dependencies"):
			_, _ = w.Write([]byte(`{"components":[{"purl":"pkg:npm/left-pad","version":"1.3.0"}],"status":{"status":"success"}}`))
		case strings.Contains(r.URL.Path, "/vulnerabilities"):
			_, _ = w.Write([]byte(`{"components":[{"purl":"pkg:npm/lodash","vulnerabilities":[{"id":"CVE-2021-23337","severity":"HIGH","source":"NVD"}]},{"purl":"pkg:npm/left-pad","vulnerabilities":[{"id":"CVE-2099-0001","severity":"LOW","source":"NVD"}]}]}`))
		case strings.Contains(r.URL.Path, "/licenses"):
			_, _ = w.Write([]byte(`{"components":[{"purl":"pkg:npm/lodash","requirement":"4.17.20","licenses":[{"id":"MIT"}]}]}`))
		case strings.Contains(r.URL.Path, "/cryptography/algorithms"):
			_, _ = w.Write([]byte(`{"components":[{"purl":"pkg:npm/lodash","requirement":"4.17.20","algorithms":[{"algorithm":"aes","strength":"256"}]}],"status":{"status":"success"}}`))
		case strings.Contains(r.URL.Path, "/geoprovenance/origin"):
			_, _ = w.Write([]byte(`{"components_locations":[{"purl":"pkg:npm/lodash","locations":[{"name":"US","percentage":80}]}],"status":{"status":"success"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func detectedResult() *scanossapi.ScanResult {
	return &scanossapi.ScanResult{
		Files: []scanossapi.FileResult{
			{Path: "a.go", MatchType: "file", Matches: []scanossapi.MatchResult{{UrlHash: "h1"}}},
		},
		Components: map[string]scanossapi.ComponentResult{
			"h1": {Purls: []string{"pkg:npm/lodash"}, Vendor: "lodash", Version: "4.17.20"},
		},
	}
}

// declaredLeftPad is the parsed-manifest input for a single declared dependency.
func declaredLeftPad() *parsers.LocalDependencies {
	return &parsers.LocalDependencies{
		Files: []parsers.LocalDependency{
			{File: "package.json", Purls: []parsers.LocalPurl{{Purl: "pkg:npm/left-pad", Requirement: "^1.3.0"}}},
		},
	}
}

func findComponent(inv sbom.Inventory, purl string) *sbom.Component {
	for i := range inv.Components {
		if inv.Components[i].Purl == purl {
			return &inv.Components[i]
		}
	}
	return nil
}

func hasVuln(inv sbom.Inventory, id string) bool {
	for _, v := range inv.Vulnerabilities {
		if v.ID == id {
			return true
		}
	}
	return false
}

func TestBuildEnrichesDetected(t *testing.T) {
	srv := decorationServer()
	defer srv.Close()
	client := scanoss.New(scanoss.WithAPIURL(srv.URL), scanoss.WithAPIKey("x"))

	inv, err := Build(context.Background(), client, detectedResult(), []scanoss.Service{scanoss.ServiceLicenses, scanoss.ServiceVulnerabilities}, false, nil, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	lodash := findComponent(inv, "pkg:npm/lodash")
	if lodash == nil {
		t.Fatal("detected component pkg:npm/lodash missing")
	}
	if len(lodash.Licenses) != 1 || lodash.Licenses[0].ID != "MIT" {
		t.Errorf("expected MIT license on lodash, got %v", lodash.Licenses)
	}
	if !hasVuln(inv, "CVE-2021-23337") {
		t.Errorf("expected CVE-2021-23337, got %v", inv.Vulnerabilities)
	}
}

// TestEnrichHandBuiltInventory proves the exported Enrich decorates an inventory that never came
// from a scan — the enrich command's entry point (parse an SBOM into an Inventory, then Enrich it
// with no fingerprinting or scan). Layers attach by PURL just as on the scan path.
func TestEnrichHandBuiltInventory(t *testing.T) {
	srv := decorationServer()
	defer srv.Close()
	client := scanoss.New(scanoss.WithAPIURL(srv.URL), scanoss.WithAPIKey("x"))

	inv := sbom.Inventory{Components: []sbom.Component{
		{Purl: "pkg:npm/lodash", Version: "4.17.20"},
	}}
	Enrich(context.Background(), client, &inv, []scanoss.Service{scanoss.ServiceLicenses, scanoss.ServiceVulnerabilities}, nil)

	lodash := findComponent(inv, "pkg:npm/lodash")
	if lodash == nil {
		t.Fatal("component pkg:npm/lodash missing")
	}
	if len(lodash.Licenses) != 1 || lodash.Licenses[0].ID != "MIT" {
		t.Errorf("expected MIT license on lodash, got %v", lodash.Licenses)
	}
	if !hasVuln(inv, "CVE-2021-23337") {
		t.Errorf("expected CVE-2021-23337, got %v", inv.Vulnerabilities)
	}
}

// TestBuildLayersAreOptIn proves no purl-layer is gathered unless requested: a bare Build
// enriches nothing, so a plain scan does no decoration work.
func TestBuildLayersAreOptIn(t *testing.T) {
	srv := decorationServer()
	defer srv.Close()
	client := scanoss.New(scanoss.WithAPIURL(srv.URL), scanoss.WithAPIKey("x"))

	inv, err := Build(context.Background(), client, detectedResult(), nil, false, nil, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	lodash := findComponent(inv, "pkg:npm/lodash")
	if lodash == nil {
		t.Fatal("detected component pkg:npm/lodash missing")
	}
	if len(lodash.Licenses) != 0 {
		t.Errorf("licenses must not be gathered without --include licenses, got %v", lodash.Licenses)
	}
	if len(inv.Vulnerabilities) != 0 {
		t.Errorf("vulnerabilities must not be gathered without --include vulns, got %v", inv.Vulnerabilities)
	}
}

// TestBuildDepsDrivenByLayer proves declared dependencies are gathered only when the deps
// layer is requested — never implicitly.
func TestBuildDepsDrivenByLayer(t *testing.T) {
	srv := decorationServer()
	defer srv.Close()
	client := scanoss.New(scanoss.WithAPIURL(srv.URL), scanoss.WithAPIKey("x"))

	// deps NOT requested: the declared input is ignored.
	inv, err := Build(context.Background(), client, detectedResult(), nil, false, declaredLeftPad(), nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if findComponent(inv, "pkg:npm/left-pad") != nil {
		t.Error("declared dependency must not appear without --include deps")
	}

	// deps requested: the declared dependency is sourced straight from the manifest (no service
	// call), so a package.json-only entry keeps its version range rather than a resolved version.
	inv, err = Build(context.Background(), client, detectedResult(), nil, true, declaredLeftPad(), nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	leftPad := findComponent(inv, "pkg:npm/left-pad")
	if leftPad == nil {
		t.Fatal("declared dependency pkg:npm/left-pad missing with --include deps")
	}
	if !leftPad.IsDeclared() {
		t.Errorf("left-pad should have declared scope, got %q", leftPad.Scope)
	}
	if leftPad.Version != "^1.3.0" {
		t.Errorf("expected the manifest requirement ^1.3.0 (no service resolution), got %q", leftPad.Version)
	}
	if len(leftPad.Evidence) != 1 || leftPad.Evidence[0].Path != "package.json" || leftPad.Evidence[0].MatchType != "declared" {
		t.Errorf("expected declared evidence from package.json, got %+v", leftPad.Evidence)
	}
}

// TestBuildDeclaredEnrichedForVulns proves declared dependencies are sourced before the
// decoration pipeline, so requesting vulns enriches them alongside the scan matches.
func TestBuildDeclaredEnrichedForVulns(t *testing.T) {
	srv := decorationServer()
	defer srv.Close()
	client := scanoss.New(scanoss.WithAPIURL(srv.URL), scanoss.WithAPIKey("x"))

	inv, err := Build(context.Background(), client, detectedResult(), []scanoss.Service{scanoss.ServiceVulnerabilities}, true, declaredLeftPad(), nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	var forDetected, forDeclared bool
	for _, v := range inv.Vulnerabilities {
		for _, p := range v.Purls {
			if p == "pkg:npm/lodash" && v.ID == "CVE-2021-23337" {
				forDetected = true
			}
			if p == "pkg:npm/left-pad" && v.ID == "CVE-2099-0001" {
				forDeclared = true
			}
		}
	}
	if !forDetected {
		t.Error("expected the detected component's vulnerability")
	}
	if !forDeclared {
		t.Error("expected the declared dependency to be enriched with its vulnerability")
	}
}

// TestBuildDepsWithoutDetectedMatches proves a project with only declared dependencies (no
// scan matches) still yields those components.
func TestBuildDepsWithoutDetectedMatches(t *testing.T) {
	srv := decorationServer()
	defer srv.Close()
	client := scanoss.New(scanoss.WithAPIURL(srv.URL), scanoss.WithAPIKey("x"))

	inv, err := Build(context.Background(), client, &scanossapi.ScanResult{}, nil, true, declaredLeftPad(), nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(inv.Components) != 1 || inv.Components[0].Purl != "pkg:npm/left-pad" {
		t.Fatalf("expected only the declared dependency, got %+v", inv.Components)
	}
	if !inv.Components[0].IsDeclared() {
		t.Errorf("the dependency should have declared scope, got %q", inv.Components[0].Scope)
	}
}

// TestSourceDeclared proves declared components are sourced from the parsed manifests without a
// service call: each PURL is split into base + version, scoped npm PURLs split correctly, and only
// EXACT duplicates (same PURL + version) are collapsed. A declared range (package.json "^1.3.0")
// is kept alongside its resolved pin (package-lock.json "1.3.0") — those are not duplicates.
func TestSourceDeclared(t *testing.T) {
	declared := &parsers.LocalDependencies{
		Files: []parsers.LocalDependency{
			{File: "package.json", Purls: []parsers.LocalPurl{
				{Purl: "pkg:npm/left-pad", Requirement: "^1.3.0"}, // range (manifest)
				{Purl: "pkg:npm/dup@1.0.0", Requirement: "1.0.0"}, // exact dup (repeated below)
			}},
			{File: "package-lock.json", Purls: []parsers.LocalPurl{
				{Purl: "pkg:npm/left-pad@1.3.0", Requirement: "1.3.0"},   // pinned — kept ALONGSIDE the range
				{Purl: "pkg:npm/@scope/pkg@2.1.0", Requirement: "2.1.0"}, // scoped, pinned
				{Purl: "pkg:npm/dup@1.0.0", Requirement: "1.0.0"},        // exact duplicate → collapsed
				{Purl: "pkg:npm/multi@1.0.0", Requirement: "1.0.0"},      // two concrete versions...
				{Purl: "pkg:npm/multi@2.0.0", Requirement: "2.0.0"},      // ...both kept
			}},
		},
	}

	got := sourceDeclared(declared, "")
	byKey := make(map[string]sbom.Component, len(got))
	for _, c := range got {
		if _, dup := byKey[c.Purl+"@"+c.Version]; dup {
			t.Errorf("duplicate entry for %s@%s", c.Purl, c.Version)
		}
		byKey[c.Purl+"@"+c.Version] = c
		if !c.IsDeclared() {
			t.Errorf("%s should have declared scope", c.Purl)
		}
	}

	for _, w := range []string{
		"pkg:npm/left-pad@^1.3.0",  // range KEPT (not dropped)
		"pkg:npm/left-pad@1.3.0",   // pinned KEPT alongside the range
		"pkg:npm/@scope/pkg@2.1.0", // scoped split correctly
		"pkg:npm/multi@1.0.0",      // both concrete versions survive
		"pkg:npm/multi@2.0.0",
		"pkg:npm/dup@1.0.0", // exact duplicate collapsed to one
	} {
		c, ok := byKey[w]
		if !ok {
			t.Errorf("missing %q in %v", w, byKey)
			continue
		}
		if len(c.Evidence) == 0 || c.Evidence[0].MatchType != "declared" {
			t.Errorf("%s should carry declared evidence, got %+v", w, c.Evidence)
		}
	}

	// dup@1.0.0 was declared in two manifests → the collapse keeps one occurrence per manifest.
	if dup := byKey["pkg:npm/dup@1.0.0"]; len(dup.Evidence) != 2 {
		t.Errorf("dup@1.0.0 should keep both manifest occurrences, got %+v", dup.Evidence)
	}
}

// TestBuildEnrichesAllPurlLayers proves the crypto and geo layers are gathered and attached
// inline on the component when requested.
func TestBuildEnrichesAllPurlLayers(t *testing.T) {
	srv := decorationServer()
	defer srv.Close()
	client := scanoss.New(scanoss.WithAPIURL(srv.URL), scanoss.WithAPIKey("x"))

	inv, err := Build(context.Background(), client, detectedResult(), []scanoss.Service{scanoss.ServiceCryptographyAlgorithms, scanoss.ServiceGeoprovenanceOrigin}, false, nil, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	lodash := findComponent(inv, "pkg:npm/lodash")
	if lodash == nil {
		t.Fatal("detected component pkg:npm/lodash missing")
	}
	if len(lodash.Cryptography) != 1 || lodash.Cryptography[0].Algorithm != "aes" {
		t.Errorf("expected the aes algorithm, got %v", lodash.Cryptography)
	}
	if len(lodash.Geoprovenance) != 1 || lodash.Geoprovenance[0].Name != "US" {
		t.Errorf("expected the US origin, got %v", lodash.Geoprovenance)
	}
}

// TestDedupeComponents proves duplicate identities (PURL+version) collapse to one — so SBOM ids
// stay unique — while distinct versions of the same PURL are kept, and a detected/declared
// overlap merges into a single detected component that combines the file-match and manifest
// evidence.
func TestDedupeComponents(t *testing.T) {
	in := []sbom.Component{
		{Purl: "pkg:npm/lodash", Version: "4.17.21", Scope: sbom.ScopeDetected, Evidence: []sbom.FileEvidence{{Path: "a.js", MatchType: "file"}}},
		{Purl: "pkg:npm/lodash", Version: "4.17.21", Scope: sbom.ScopeDeclared, Evidence: []sbom.FileEvidence{{Path: "package.json", MatchType: "declared"}}}, // dup of the above
		{Purl: "pkg:npm/abort-controller", Version: "3.0.0", Scope: sbom.ScopeDeclared, Evidence: []sbom.FileEvidence{{Path: "package.json", MatchType: "declared"}}},
		{Purl: "pkg:npm/abort-controller", Version: "3.0.0", Scope: sbom.ScopeDeclared, Evidence: []sbom.FileEvidence{{Path: "package-lock.json", MatchType: "declared"}}}, // exact dup — evidence merged
		{Purl: "pkg:npm/tar", Version: "6.2.0", Scope: sbom.ScopeDeclared},
		{Purl: "pkg:npm/tar", Version: "7.5.7", Scope: sbom.ScopeDeclared}, // same purl, different version — kept
	}

	out := dedupeComponents(in)
	if len(out) != 4 {
		t.Fatalf("got %d components, want 4: %+v", len(out), out)
	}

	// lodash: detected wins, and the file-match + manifest evidence are combined.
	lodash := out[0]
	if lodash.Scope != sbom.ScopeDetected || len(lodash.Evidence) != 2 {
		t.Errorf("lodash merge wrong (want detected with 2 evidences): %+v", lodash)
	}

	// tar keeps both versions.
	var tarVersions []string
	for _, c := range out {
		if c.Purl == "pkg:npm/tar" {
			tarVersions = append(tarVersions, c.Version)
		}
	}
	if len(tarVersions) != 2 {
		t.Errorf("tar should keep both versions, got %v", tarVersions)
	}

	// every (purl,version) is unique.
	seen := map[string]bool{}
	for _, c := range out {
		k := c.Purl + "@" + c.Version
		if seen[k] {
			t.Errorf("duplicate identity survived: %s", k)
		}
		seen[k] = true
	}
}
