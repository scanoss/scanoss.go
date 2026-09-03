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
	"sort"

	scanossapi "github.com/scanoss/scanoss.api-sdk"

	"github.com/scanoss/scanoss.go/pkg/settings"
)

// How strongly a bom.identify rule claims a match, mirroring the scan engine's asset_declared:
// naming the component earns one step, naming its exact version earns two. A rule that pinned
// the version is a stronger statement than one that named the component alone, and when two
// components match one file that difference is what decides which of them leads.
const (
	identifiedNone = iota
	identifiedPurl
	identifiedPurlVersion
)

// Report is what the selection rules concluded, for consumers that need more than the rewritten
// result. The scan result itself cannot carry it: its types come from the API SDK, which has no
// field for a client-side verdict.
type Report struct {
	// Identified holds the url_hash of every component a bom.identify rule claimed, for at
	// least one file. A path-scoped rule that claimed one of a component's files puts it here:
	// the user declared that component present, whatever the scope of the rule that said so.
	Identified map[string]bool

	// Evidence records the claim per file, keyed by the scanned path and the matched
	// component's url_hash. It is the finer answer Identified summarises, and the one a
	// per-file report needs: a rule scoped to "vendor/" claims a component in the files under
	// it and not in the ones outside, which a component-level flag cannot say.
	Evidence map[EvidenceKey]bool
}

// EvidenceKey identifies one match: the scanned file, and the component it matched.
type EvidenceKey struct {
	Path    string
	URLHash string
}

// IsIdentified reports whether a bom.identify rule claimed the component matched at this path.
// It is nil-safe, so a caller holding a zero Report can ask without checking first — which is
// what lets it be passed as a function to consumers that must not depend on this package.
func (r Report) IsIdentified(path, urlHash string) bool {
	return r.Evidence[EvidenceKey{Path: path, URLHash: urlHash}]
}

// applySelect settles which of the candidate matches the batch scanner returned for each file
// survive, and which of them leads, in a single pass. It applies the two match-level BOM rules —
// bom.ignore and bom.identify — and the ranking threshold, and reports which components were
// identified.
//
// The three are one traversal because they rewrite one thing: a file's candidate list. bom.ignore
// drops candidates the user does not want considered; the ranking threshold drops the ones the
// scanner ranked too weakly; bom.identify promotes the candidate the user declares is really
// there, so it leads the list. Splitting them would mean walking every file three times and
// pruning the component catalog three times, and would leave their order as a comment for
// someone to reverse.
//
// One rule decides every interaction between them: a match an identify rule claims is never
// dropped, by either filter. It mirrors how bom.include protects a file from bom.remove — an
// explicit "this component is here" outranks both a blanket "drop this one" and a generic
// quality bar.
//
// That is a deliberate divergence from the engine, which applies the threshold last and lets it
// win: url.c overwrites the identified flag asset_declared had just set with IDENTIFIED_FILTERED.
// Reproducing it would discard a component the user stated was present because of a rule that
// names no component at all.
//
// Matches that no rule touches keep the order the server gave them: the scan already ranked them,
// and nothing here knows better.
//
// A nil result, or nothing to apply, is a no-op returning an empty Report.
func applySelect(result *scanossapi.ScanResult, bom *settings.BOM, rankingThreshold int) Report {
	if result == nil {
		return Report{}
	}
	var identifyRules, ignoreRules []settings.BOMEntry
	if bom != nil {
		identifyRules, ignoreRules = bom.IdentifyRules(), bom.IgnoreRules()
	}
	// At or below zero the threshold is off, as it is in the engine, whose filter is guarded by
	// component_rank_max > 0. -1 means "defer to the server", which for a client-side filter is
	// the same as off.
	if rankingThreshold < 0 {
		rankingThreshold = 0
	}
	if len(identifyRules) == 0 && len(ignoreRules) == 0 && rankingThreshold == 0 {
		return Report{}
	}

	report := Report{
		Identified: make(map[string]bool),
		Evidence:   make(map[EvidenceKey]bool),
	}
	dropped := false

	for i := range result.Files {
		f := &result.Files[i]
		if f.MatchType == "" || f.MatchType == "none" {
			continue
		}

		kept := make([]scoredMatch, 0, len(f.Matches))
		for _, m := range f.Matches {
			comp := result.Components[m.UrlHash]
			score := identifyStrength(identifyRules, f.Path, comp)

			if score > identifiedNone {
				report.Identified[m.UrlHash] = true
				report.Evidence[EvidenceKey{Path: f.Path, URLHash: m.UrlHash}] = true
			} else if shouldIgnore(f.Path, comp, ignoreRules) || outranked(comp, rankingThreshold) {
				dropped = true
				continue
			}
			kept = append(kept, scoredMatch{match: m, score: score})
		}

		// Every candidate was ignored: the file is left as the scan reports an unmatched one.
		// Path and hashes stay, as they do for a file neutralized by bom.remove.
		if len(kept) == 0 {
			f.MatchType = "none"
			f.Matches = nil
			continue
		}

		// Stable, so matches of equal standing keep the server's ordering.
		sort.SliceStable(kept, func(a, b int) bool { return kept[a].score > kept[b].score })
		f.Matches = make([]scanossapi.MatchResult, len(kept))
		for j, k := range kept {
			f.Matches[j] = k.match
		}
	}

	// Only ignore removes references, so only ignore can orphan a catalog entry.
	if dropped {
		pruneUnreferencedComponents(result)
	}
	return report
}

// scoredMatch pairs a match with how strongly bom.identify claims it, so the two travel together
// through the sort that puts the declared component first.
type scoredMatch struct {
	match scanossapi.MatchResult
	score int
}

// matchPurls returns every PURL of the component one match points at — its canonical one and its
// aliases — by joining its url_hash to the component catalog. It is what the file-level rules
// (bom.remove, bom.replace) compare against.
//
// The match-level rules deliberately do not use it; see canonicalPurl.
func matchPurls(m scanossapi.MatchResult, components map[string]scanossapi.ComponentResult) []string {
	return components[m.UrlHash].Purls
}

// canonicalPurl returns the one PURL that identifies a component, ignoring its aliases.
//
// The alias list is not a list of other components: one upstream project mined from several
// registries yields catalog entries that each carry the others' PURLs. Scanning zephyr's "west"
// returns three components whose alias lists all name pkg:pypi/west, pkg:github/…/west and
// pkg:conda/west.
//
// So the match-level rules compare against this and not the whole list, which is what the engine
// does too (asset_declared reads comp->purls[0]; ignored_asset_match reads the record's own
// purl). Comparing against aliases would make one rule claim every candidate at once — which
// defeats bom.identify, whose entire job is to pick which candidate is the right one, and makes
// bom.ignore drop components the user never named.
func canonicalPurl(comp scanossapi.ComponentResult) string {
	if len(comp.Purls) == 0 {
		return ""
	}
	return comp.Purls[0]
}

// claimsComponent reports whether a rule's PURL names this component, comparing without versions
// so a rule written against a specific release still covers it.
func claimsComponent(rule settings.BOMEntry, comp scanossapi.ComponentResult) bool {
	canonical := canonicalPurl(comp)
	return canonical != "" && stripVersion(canonical) == stripVersion(rule.Purl)
}

// identifyStrength returns the strongest claim any bom.identify rule makes on one match, or
// identifiedNone when none of them claims it.
func identifyStrength(rules []settings.BOMEntry, filePath string, comp scanossapi.ComponentResult) int {
	best := identifiedNone
	for _, rule := range rules {
		if score := identifyScore(rule, filePath, comp); score > best {
			best = score
		}
	}
	return best
}

// identifyScore rates how strongly one bom.identify rule claims one match.
//
// A rule naming no PURL is ignored rather than applied: identify has to say which of a file's
// candidate components is the right one, and a rule that names only a path says nothing about
// that. (bom.remove, which acts on whole files, does accept a path-only rule — there the path
// alone is the whole statement.)
//
// A rule that pins a version the match does not carry still counts as naming the component: the
// user declared it present, they were only wrong about which release. That is the engine's
// behaviour in asset_declared, and dropping to no claim at all would silently ignore the rule.
func identifyScore(rule settings.BOMEntry, filePath string, comp scanossapi.ComponentResult) int {
	if rule.Purl == "" {
		return identifiedNone
	}
	if rule.Path != "" && !matchPath(rule.Path, filePath) {
		return identifiedNone
	}
	if !claimsComponent(rule, comp) {
		return identifiedNone
	}
	if _, ruleVersion := splitPurlVersion(rule.Purl); ruleVersion != "" && ruleVersion == comp.Version {
		return identifiedPurlVersion
	}
	return identifiedPurl
}

// unrankedComponent is the rank the engine gives a component it has no ranking information for
// (COMPONENT_DEFAULT_RANK in component.h). It is a sentinel, not a bad rank: the engine's own
// "accept everything" bound is this value plus one, so an unranked component passes it.
const unrankedComponent = 999

// outranked reports whether a component ranks worse than the threshold, and so is not worth
// reporting. Rank is the scanner's own ordering of how well a component explains a match, lowest
// is strongest; real ranks run 1..9.
//
// A threshold of 0 means the filter is off. Two rank values mean "not ranked" rather than
// "ranked badly", and neither is filtered: 0, because the field is omitted when empty and the
// client cannot tell an absent rank from a zero one; and 999, the engine's sentinel for a
// component it has no ranking information about. Filtering either would discard a component for
// missing data rather than for explaining a match poorly — and 999 exceeds every threshold, so
// it would be discarded by all of them.
func outranked(comp scanossapi.ComponentResult, threshold int) bool {
	if threshold <= 0 || comp.Rank <= 0 || comp.Rank >= unrankedComponent {
		return false
	}
	return comp.Rank > threshold
}

// shouldIgnore reports whether a bom.ignore rule covers one match. PURLs are compared without
// versions, so a rule written against a specific release still covers the component — more
// forgiving than the engine, whose ignored_asset_match uses a plain string equality against the
// versionless PURL, so a rule written as "pkg:npm/vue@2.6.12" matches nothing there at all.
func shouldIgnore(filePath string, comp scanossapi.ComponentResult, rules []settings.BOMEntry) bool {
	for _, rule := range rules {
		if rule.Purl == "" {
			continue // an ignore rule that names no component would drop every match
		}
		if rule.Path != "" && !matchPath(rule.Path, filePath) {
			continue
		}
		if claimsComponent(rule, comp) {
			return true
		}
	}
	return false
}
