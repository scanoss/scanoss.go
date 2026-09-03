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

package postprocess

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	scanossapi "github.com/scanoss/scanoss.api-sdk"

	"github.com/scanoss/scanoss.go/pkg/settings"
)

// selectFixture is one file the scanner returned three candidates for, in its own ranking order:
// vue, then lodash, then jquery.
func selectFixture() *scanossapi.ScanResult {
	return &scanossapi.ScanResult{
		Files: []scanossapi.FileResult{{
			Path:       "src/app.js",
			MatchType:  "file",
			SourceHash: "src-hash",
			FileHash:   "file-hash",
			Matches: []scanossapi.MatchResult{
				{UrlHash: "vue", OssFilePath: "vue.js"},
				{UrlHash: "lodash", OssFilePath: "lodash.js"},
				{UrlHash: "jquery", OssFilePath: "jquery.js"},
			},
		}},
		Components: map[string]scanossapi.ComponentResult{
			"vue":    {Purls: []string{"pkg:npm/vue"}, Component: "vue", Version: "2.6.14"},
			"lodash": {Purls: []string{"pkg:npm/lodash"}, Component: "lodash", Version: "4.17.21"},
			"jquery": {Purls: []string{"pkg:npm/jquery"}, Component: "jquery", Version: "3.6.0"},
		},
	}
}

// selectRules runs the match-level rules with the ranking filter off, which is what every test
// that is not about ranking wants.
func selectRules(res *scanossapi.ScanResult, bom *settings.BOM) Report {
	return applySelect(res, bom, 0)
}

// hashOrder is the url_hash of each surviving match, in order.
func hashOrder(res *scanossapi.ScanResult, file int) []string {
	out := make([]string, 0, len(res.Files[file].Matches))
	for _, m := range res.Files[file].Matches {
		out = append(out, m.UrlHash)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSelectPromotesTheDeclaredComponent(t *testing.T) {
	res := selectFixture()
	report := selectRules(res, &settings.BOM{Identify: []settings.BOMEntry{
		{Purl: "pkg:npm/lodash"},
	}})

	if got, want := hashOrder(res, 0), []string{"lodash", "vue", "jquery"}; !equalStrings(got, want) {
		t.Errorf("match order = %v, want %v", got, want)
	}
	if !report.Identified["lodash"] {
		t.Error("lodash should be reported as identified")
	}
	if report.Identified["vue"] || report.Identified["jquery"] {
		t.Errorf("only lodash was declared, got %v", report.Identified)
	}
}

// The candidates the user said nothing about keep the order the scan gave them: it already
// ranked them, and nothing here knows better.
func TestSelectKeepsServerOrderForUnclaimedMatches(t *testing.T) {
	res := selectFixture()
	selectRules(res, &settings.BOM{Identify: []settings.BOMEntry{
		{Purl: "pkg:npm/jquery"},
	}})

	if got, want := hashOrder(res, 0), []string{"jquery", "vue", "lodash"}; !equalStrings(got, want) {
		t.Errorf("match order = %v, want %v: vue and lodash should stay in scan order", got, want)
	}
}

// Mirrors the engine's asset_declared: naming the exact version is a stronger claim than naming
// the component alone, so when two candidates are both declared the pinned one leads.
func TestSelectPrefersThePinnedVersion(t *testing.T) {
	res := selectFixture()
	selectRules(res, &settings.BOM{Identify: []settings.BOMEntry{
		{Purl: "pkg:npm/vue"},          // names the component
		{Purl: "pkg:npm/jquery@3.6.0"}, // names its exact version
	}})

	if got := hashOrder(res, 0)[0]; got != "jquery" {
		t.Errorf("leading match = %q, want jquery: a pinned version outranks a bare PURL", got)
	}
}

// A rule that pinned the wrong release still declares the component present. The engine returns
// IDENTIFIED_PURL there rather than dropping to no claim, and so does this.
func TestSelectCountsAWrongVersionAsAClaim(t *testing.T) {
	res := selectFixture()
	report := selectRules(res, &settings.BOM{Identify: []settings.BOMEntry{
		{Purl: "pkg:npm/lodash@1.0.0"}, // fixture carries 4.17.21
	}})

	if got := hashOrder(res, 0)[0]; got != "lodash" {
		t.Errorf("leading match = %q, want lodash", got)
	}
	if !report.Identified["lodash"] {
		t.Error("lodash should still be reported as identified")
	}
}

func TestSelectDropsIgnoredMatches(t *testing.T) {
	res := selectFixture()
	selectRules(res, &settings.BOM{Ignore: []settings.BOMEntry{
		{Purl: "pkg:npm/vue"},
	}})

	if got, want := hashOrder(res, 0), []string{"lodash", "jquery"}; !equalStrings(got, want) {
		t.Errorf("matches = %v, want %v", got, want)
	}
	if _, ok := res.Components["vue"]; ok {
		t.Error("the vue catalog entry should have been pruned once nothing referenced it")
	}
}

// A rule written against one release still covers the component — more forgiving than the
// engine, whose ignored_asset_match would match nothing for this rule.
func TestSelectIgnoreMatchesRegardlessOfVersion(t *testing.T) {
	res := selectFixture()
	selectRules(res, &settings.BOM{Ignore: []settings.BOMEntry{
		{Purl: "pkg:npm/vue@2.6.14"},
	}})

	if got, want := hashOrder(res, 0), []string{"lodash", "jquery"}; !equalStrings(got, want) {
		t.Errorf("matches = %v, want %v", got, want)
	}
}

func TestSelectIgnoreRespectsPathScope(t *testing.T) {
	res := selectFixture()
	selectRules(res, &settings.BOM{Ignore: []settings.BOMEntry{
		{Purl: "pkg:npm/vue", Path: "vendor/"},
	}})

	if got := len(res.Files[0].Matches); got != 3 {
		t.Errorf("got %d matches, want 3: the rule is scoped to a path the file is not under", got)
	}
}

// Every candidate ignored leaves the file as the scan reports an unmatched one, the way
// bom.remove neutralizes a file.
func TestSelectIgnoringEveryMatchNeutralizesTheFile(t *testing.T) {
	res := selectFixture()
	selectRules(res, &settings.BOM{Ignore: []settings.BOMEntry{
		{Purl: "pkg:npm/vue"}, {Purl: "pkg:npm/lodash"}, {Purl: "pkg:npm/jquery"},
	}})

	f := res.Files[0]
	if f.MatchType != "none" {
		t.Errorf("match_type = %q, want none", f.MatchType)
	}
	if len(f.Matches) != 0 {
		t.Errorf("matches = %v, want none", f.Matches)
	}
	if f.Path != "src/app.js" {
		t.Errorf("path = %q, want it preserved", f.Path)
	}
	if len(res.Components) != 0 {
		t.Errorf("components = %v, want all pruned", res.Components)
	}
}

// A settings file that both declares a component and ignores it is stating a contradiction.
// Identify wins, the way bom.include protects a file from bom.remove.
func TestSelectIdentifyProtectsFromIgnore(t *testing.T) {
	res := selectFixture()
	report := selectRules(res, &settings.BOM{
		Identify: []settings.BOMEntry{{Purl: "pkg:npm/vue"}},
		Ignore:   []settings.BOMEntry{{Purl: "pkg:npm/vue"}},
	})

	if got := hashOrder(res, 0)[0]; got != "vue" {
		t.Errorf("leading match = %q, want vue: an identify rule outranks an ignore rule", got)
	}
	if !report.Identified["vue"] {
		t.Error("vue should be reported as identified")
	}
}

// Both spellings of each rule reach applySelect, since a settings file may use either.
func TestSelectReadsBothRuleSpellings(t *testing.T) {
	res := selectFixture()
	report := selectRules(res, &settings.BOM{
		Include: []settings.BOMEntry{{Purl: "pkg:npm/lodash"}}, // schema spelling of identify
		Exclude: []settings.BOMEntry{{Purl: "pkg:npm/vue"}},    // schema spelling of ignore
	})

	if got, want := hashOrder(res, 0), []string{"lodash", "jquery"}; !equalStrings(got, want) {
		t.Errorf("matches = %v, want %v", got, want)
	}
	if !report.Identified["lodash"] {
		t.Error("lodash should be reported as identified via bom.include")
	}
}

// Identify has to say which of a file's candidates is the right one; a rule naming only a path
// says nothing about that, so it claims nothing rather than claiming everything under it.
func TestSelectIgnoresPathOnlyRules(t *testing.T) {
	res := selectFixture()
	report := selectRules(res, &settings.BOM{
		Identify: []settings.BOMEntry{{Path: "src/"}},
		Ignore:   []settings.BOMEntry{{Path: "src/"}},
	})

	if got := len(res.Files[0].Matches); got != 3 {
		t.Errorf("got %d matches, want all 3 untouched", got)
	}
	if len(report.Identified) != 0 {
		t.Errorf("identified = %v, want none", report.Identified)
	}
}

func TestSelectSkipsUnmatchedFiles(t *testing.T) {
	res := &scanossapi.ScanResult{
		Files: []scanossapi.FileResult{{Path: "src/none.js", MatchType: "none"}},
	}
	selectRules(res, &settings.BOM{Ignore: []settings.BOMEntry{{Purl: "pkg:npm/vue"}}})

	if res.Files[0].MatchType != "none" {
		t.Errorf("match_type = %q, want none", res.Files[0].MatchType)
	}
}

func TestSelectNoOps(t *testing.T) {
	cases := map[string]*settings.BOM{
		"nil BOM":     nil,
		"empty BOM":   {},
		"other rules": {Remove: []settings.BOMEntry{{Purl: "pkg:npm/vue"}}},
	}
	for name, bom := range cases {
		t.Run(name, func(t *testing.T) {
			res := selectFixture()
			report := selectRules(res, bom)
			if got, want := hashOrder(res, 0), []string{"vue", "lodash", "jquery"}; !equalStrings(got, want) {
				t.Errorf("matches = %v, want them untouched (%v)", got, want)
			}
			if len(report.Identified) != 0 {
				t.Errorf("identified = %v, want none", report.Identified)
			}
		})
	}

	selectRules(nil, &settings.BOM{Ignore: []settings.BOMEntry{{Purl: "pkg:npm/vue"}}})
}

// Apply settles the candidate list before the file-level rules run, so bom.remove sees what the
// user actually meant. Here vue would have neutralized the file, but it was ignored first.
func TestApplySelectsBeforeRemoving(t *testing.T) {
	res := selectFixture()
	Apply(res, &settings.BOM{
		Ignore: []settings.BOMEntry{{Purl: "pkg:npm/vue"}},
		Remove: []settings.BOMEntry{{Purl: "pkg:npm/vue"}},
	})

	if res.Files[0].MatchType == "none" {
		t.Error("the file should survive: vue was dropped from the candidates before remove ran")
	}
	if got, want := hashOrder(res, 0), []string{"lodash", "jquery"}; !equalStrings(got, want) {
		t.Errorf("matches = %v, want %v", got, want)
	}
}

func TestApplyReturnsTheSelectionReport(t *testing.T) {
	res := selectFixture()
	report := Apply(res, &settings.BOM{Identify: []settings.BOMEntry{{Purl: "pkg:npm/vue"}}})

	if !report.Identified["vue"] {
		t.Errorf("identified = %v, want vue", report.Identified)
	}
}

// bom.identify protects from bom.remove exactly as bom.include does: the two are one rule
// under two names, and which word a settings file used must not change the outcome.
func TestRemoveProtectionHonorsBothIdentifySpellings(t *testing.T) {
	for _, spelling := range []string{"identify", "include"} {
		t.Run(spelling, func(t *testing.T) {
			bom := &settings.BOM{Remove: []settings.BOMEntry{{Purl: "pkg:npm/vue"}}}
			entry := []settings.BOMEntry{{Purl: "pkg:npm/vue"}}
			if spelling == "identify" {
				bom.Identify = entry
			} else {
				bom.Include = entry
			}

			res := &scanossapi.ScanResult{
				Files: []scanossapi.FileResult{{
					Path: "src/app.js", MatchType: "file",
					Matches: []scanossapi.MatchResult{{UrlHash: "vue"}},
				}},
				Components: map[string]scanossapi.ComponentResult{
					"vue": {Purls: []string{"pkg:npm/vue"}, Version: "2.6.14"},
				},
			}
			Apply(res, bom)

			if res.Files[0].MatchType == "none" {
				t.Errorf("bom.%s should have protected the file from bom.remove", spelling)
			}
		})
	}
}

// rankedFixture is one file with three candidates whose ranks span the threshold.
func rankedFixture() *scanossapi.ScanResult {
	return &scanossapi.ScanResult{
		Files: []scanossapi.FileResult{{
			Path: "src/app.js", MatchType: "file",
			Matches: []scanossapi.MatchResult{
				{UrlHash: "strong"}, {UrlHash: "middling"}, {UrlHash: "weak"},
			},
		}},
		Components: map[string]scanossapi.ComponentResult{
			"strong":   {Purls: []string{"pkg:npm/strong"}, Version: "1.0.0", Rank: 1},
			"middling": {Purls: []string{"pkg:npm/middling"}, Version: "1.0.0", Rank: 5},
			"weak":     {Purls: []string{"pkg:npm/weak"}, Version: "1.0.0", Rank: 9},
		},
	}
}

func TestRankingThresholdDropsWeakMatches(t *testing.T) {
	tests := []struct {
		threshold int
		want      []string
	}{
		{threshold: 1, want: []string{"strong"}},
		{threshold: 5, want: []string{"strong", "middling"}},
		{threshold: 9, want: []string{"strong", "middling", "weak"}},
		{threshold: 10, want: []string{"strong", "middling", "weak"}},
		// At or below zero the filter is off, as the engine's component_rank_max > 0 guard has
		// it. -1 means "defer to the server", which client-side is the same as off.
		{threshold: 0, want: []string{"strong", "middling", "weak"}},
		{threshold: -1, want: []string{"strong", "middling", "weak"}},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("threshold %d", tt.threshold), func(t *testing.T) {
			res := rankedFixture()
			applySelect(res, nil, tt.threshold)

			if got := hashOrder(res, 0); !equalStrings(got, tt.want) {
				t.Errorf("matches = %v, want %v", got, tt.want)
			}
			for _, hash := range tt.want {
				if _, ok := res.Components[hash]; !ok {
					t.Errorf("component %q was pruned but its match survived", hash)
				}
			}
		})
	}
}

// Dropping every candidate leaves the file as the scan reports an unmatched one, and orphans
// nothing in the catalog.
func TestRankingThresholdCanNeutralizeAFile(t *testing.T) {
	res := rankedFixture()
	applySelect(res, nil, 0) // sanity: off changes nothing
	if len(res.Files[0].Matches) != 3 {
		t.Fatal("the filter should have been off")
	}

	res = rankedFixture()
	for hash, comp := range res.Components {
		comp.Rank = 9
		res.Components[hash] = comp
	}
	applySelect(res, nil, 1)

	if res.Files[0].MatchType != "none" {
		t.Errorf("match_type = %q, want none", res.Files[0].MatchType)
	}
	if len(res.Components) != 0 {
		t.Errorf("components = %v, want all pruned", res.Components)
	}
}

// A component the user declared present is never dropped by the ranking filter. This diverges
// from the engine, which overwrites the identified flag with IDENTIFIED_FILTERED in url.c and
// lets the threshold win.
func TestRankingThresholdNeverDropsAnIdentifiedMatch(t *testing.T) {
	res := rankedFixture()
	report := applySelect(res, &settings.BOM{Identify: []settings.BOMEntry{
		{Purl: "pkg:npm/weak"}, // rank 9, far past the threshold
	}}, 1)

	// The declared component survives, and leads: identify still promotes it.
	if got, want := hashOrder(res, 0), []string{"weak", "strong"}; !equalStrings(got, want) {
		t.Errorf("matches = %v, want %v", got, want)
	}
	if !report.Identified["weak"] {
		t.Error("weak should be reported as identified")
	}
}

// Rank 0 means "not reported" — the field is omitted when empty, so the client cannot tell it
// from the strongest possible rank. Filtering nothing is the safe reading.
func TestRankingThresholdIgnoresUnrankedComponents(t *testing.T) {
	res := &scanossapi.ScanResult{
		Files: []scanossapi.FileResult{{
			Path: "src/app.js", MatchType: "file",
			Matches: []scanossapi.MatchResult{{UrlHash: "unranked"}},
		}},
		Components: map[string]scanossapi.ComponentResult{
			"unranked": {Purls: []string{"pkg:npm/x"}, Version: "1.0.0"}, // Rank left at 0
		},
	}
	applySelect(res, nil, 1)

	if len(res.Files[0].Matches) != 1 {
		t.Error("an unranked component must not be filtered")
	}
}

// The threshold reaches applySelect through Apply's option, and works with no BOM at all.
func TestApplyRankingThresholdOption(t *testing.T) {
	res := rankedFixture()
	Apply(res, nil, WithRankingThreshold(1))

	if got, want := hashOrder(res, 0), []string{"strong"}; !equalStrings(got, want) {
		t.Errorf("matches = %v, want %v", got, want)
	}
}

// Ranking filters, ignore drops and identify promotes, all in one pass.
func TestSelectCombinesEveryMatchRule(t *testing.T) {
	res := rankedFixture()
	report := applySelect(res, &settings.BOM{
		Identify: []settings.BOMEntry{{Purl: "pkg:npm/weak"}},   // rank 9: protected
		Ignore:   []settings.BOMEntry{{Purl: "pkg:npm/strong"}}, // rank 1: dropped anyway
	}, 5)

	// strong is ignored; middling passes the threshold; weak is protected and leads.
	if got, want := hashOrder(res, 0), []string{"weak", "middling"}; !equalStrings(got, want) {
		t.Errorf("matches = %v, want %v", got, want)
	}
	if !report.Identified["weak"] || len(report.Identified) != 1 {
		t.Errorf("identified = %v, want just weak", report.Identified)
	}
}

// A real captured envelope, to check the filter against the ranks the batch scanner actually
// reports rather than only the ones a fixture invents. In this one four components carry ranks
// 5, 5, 6 and 6.
func TestRankingThresholdOnACapturedScanResult(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "scanoss", "testdata", "scan_envelope_file.json"))
	if err != nil {
		t.Skipf("captured envelope unavailable: %v", err)
	}
	var envelope struct {
		Result scanossapi.ScanResult `json:"result"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	ranks := map[string]int{}
	for hash, comp := range envelope.Result.Components {
		if comp.Rank == 0 {
			t.Fatalf("component %s has no rank: this test needs ranked input", hash)
		}
		ranks[hash] = comp.Rank
	}
	if len(ranks) == 0 {
		t.Fatal("no components in the captured envelope")
	}

	result := envelope.Result
	applySelect(&result, nil, 5)

	// Everything ranked worse than 5 is gone from every file, and from the catalog.
	for hash, comp := range result.Components {
		if comp.Rank > 5 {
			t.Errorf("component %s survived with rank %d", hash, comp.Rank)
		}
	}
	for _, f := range result.Files {
		for _, m := range f.Matches {
			if ranks[m.UrlHash] > 5 {
				t.Errorf("file %s still matches %s (rank %d)", f.Path, m.UrlHash, ranks[m.UrlHash])
			}
		}
	}
	// The rank-5 components are still there: the threshold is inclusive.
	if len(result.Components) == 0 {
		t.Error("threshold 5 dropped everything, but two components rank exactly 5")
	}
}

// aliasFixture reproduces what a live scan of zephyr's "west" returns: one upstream project
// mined from three registries, so each catalog entry carries the others' PURLs as aliases.
// Ranks and alias lists are the ones the API actually returned.
func aliasFixture() *scanossapi.ScanResult {
	return &scanossapi.ScanResult{
		Files: []scanossapi.FileResult{{
			Path: "src/west/app/main.py", MatchType: "file",
			Matches: []scanossapi.MatchResult{
				{UrlHash: "pypi-old"}, {UrlHash: "github"}, {UrlHash: "pypi-new"},
			},
		}},
		Components: map[string]scanossapi.ComponentResult{
			"pypi-old": {Rank: 7, Version: "1.1.0a1", Purls: []string{
				"pkg:pypi/west", "pkg:github/zephyrproject-rtos/west", "pkg:conda/west"}},
			"github": {Rank: 5, Version: "v1.2.0", Purls: []string{
				"pkg:github/zephyrproject-rtos/west", "pkg:conda/west", "pkg:pypi/west"}},
			"pypi-new": {Rank: 7, Version: "1.2.0", Purls: []string{
				"pkg:pypi/west", "pkg:github/zephyrproject-rtos/west", "pkg:conda/west"}},
		},
	}
}

// A rule naming one registry's PURL must not claim the components whose canonical PURL is a
// different registry's. Comparing against the alias list made one rule claim all three, which
// defeats identify (it selects nothing) and makes ignore drop components the user never named.
func TestSelectMatchesTheCanonicalPurlNotAliases(t *testing.T) {
	t.Run("identify claims only the components it names", func(t *testing.T) {
		res := aliasFixture()
		report := selectRules(res, &settings.BOM{Identify: []settings.BOMEntry{
			{Purl: "pkg:pypi/west"},
		}})

		if !report.Identified["pypi-old"] || !report.Identified["pypi-new"] {
			t.Errorf("both pypi components should be identified, got %v", report.Identified)
		}
		if report.Identified["github"] {
			t.Error("the github component was not named and must not be identified")
		}
		// Identify actually selects now: the two claimed candidates lead.
		if got, want := hashOrder(res, 0), []string{"pypi-old", "pypi-new", "github"}; !equalStrings(got, want) {
			t.Errorf("match order = %v, want %v", got, want)
		}
	})

	t.Run("ignore drops only the components it names", func(t *testing.T) {
		res := aliasFixture()
		selectRules(res, &settings.BOM{Ignore: []settings.BOMEntry{
			{Purl: "pkg:pypi/west"},
		}})

		if got, want := hashOrder(res, 0), []string{"github"}; !equalStrings(got, want) {
			t.Errorf("matches = %v, want %v: the github component was never named", got, want)
		}
	})

	t.Run("a rule naming only an alias claims nothing", func(t *testing.T) {
		res := aliasFixture()
		report := selectRules(res, &settings.BOM{Identify: []settings.BOMEntry{
			{Purl: "pkg:conda/west"}, // an alias of all three, canonical for none
		}})

		if len(report.Identified) != 0 {
			t.Errorf("identified = %v, want none", report.Identified)
		}
	})

	// The exact-version bonus still resolves against the component's own version.
	t.Run("the pinned version still wins", func(t *testing.T) {
		res := aliasFixture()
		selectRules(res, &settings.BOM{Identify: []settings.BOMEntry{
			{Purl: "pkg:pypi/west@1.2.0"},
		}})

		if got := hashOrder(res, 0)[0]; got != "pypi-new" {
			t.Errorf("leading match = %q, want pypi-new (the 1.2.0 release)", got)
		}
	})
}

// The report answers per file, which is what a path-scoped rule needs: the same component is
// claimed in the files the rule covers and not in the ones outside it.
func TestReportAnswersPerFile(t *testing.T) {
	res := &scanossapi.ScanResult{
		Files: []scanossapi.FileResult{
			{Path: "vendor/vue.js", MatchType: "file", Matches: []scanossapi.MatchResult{{UrlHash: "vue"}}},
			{Path: "src/app.js", MatchType: "file", Matches: []scanossapi.MatchResult{{UrlHash: "vue"}}},
		},
		Components: map[string]scanossapi.ComponentResult{
			"vue": {Purls: []string{"pkg:npm/vue"}, Version: "2.6.14"},
		},
	}
	report := selectRules(res, &settings.BOM{Identify: []settings.BOMEntry{
		{Purl: "pkg:npm/vue", Path: "vendor/"},
	}})

	if !report.IsIdentified("vendor/vue.js", "vue") {
		t.Error("vendor/vue.js is inside the rule's scope and should be identified")
	}
	if report.IsIdentified("src/app.js", "vue") {
		t.Error("src/app.js is outside the rule's scope and must not be identified")
	}
	// The component-level summary says "claimed somewhere", which it was.
	if !report.Identified["vue"] {
		t.Error("the component should appear in the component-level summary")
	}
}

// A zero Report answers for everything without panicking, so callers can pass IsIdentified
// straight through without checking whether any rules ran.
func TestZeroReportIsQueryable(t *testing.T) {
	var report Report
	if report.IsIdentified("any/path", "any-hash") {
		t.Error("a zero Report should claim nothing")
	}
}
