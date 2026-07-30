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

package scanoss

import (
	"sort"
	"strings"

	scanossapi "github.com/scanoss/scanoss.api-sdk"

	"github.com/scanoss/scanoss.go/pkg/settings"
)

// ApplyBOMReplace applies bom.replace rules to a scan result in place: a file whose match is
// covered by a rule is re-pointed at the rule's replace_with component, so the result reports the
// component the user says is really there.
//
// It runs after ApplyBOMRemove, never before: a match that is about to be discarded is not worth
// relabelling first, and running the other way round would let a replacement smuggle a dismissed
// component back in under a new PURL.
//
// Only the component identity changes. Everything the scan observed about the file — path, hashes,
// matched line ranges, match type — is what the scan found and stays untouched. Licences,
// vulnerabilities and the rest are not here to reset either: they are gathered afterwards by the
// decoration services, which will be handed the replacement PURL and answer for that.
//
// A nil result or nil/empty rules is a no-op.
func ApplyBOMReplace(result *scanossapi.ScanResult, bom *settings.BOM) {
	if result == nil || bom == nil || len(bom.Replace) == 0 {
		return
	}

	for i := range result.Files {
		f := &result.Files[i]
		if f.MatchType == "" || f.MatchType == "none" {
			continue
		}
		rule, ok := bestReplaceRule(f.Path, filePurls(f, result.Components), bom.Replace)
		if !ok {
			continue
		}
		hash := componentForPurl(result, rule.ReplaceWith)
		for j := range f.Matches {
			f.Matches[j].UrlHash = hash
		}
	}

	pruneUnreferencedComponents(result)
}

// bestReplaceRule returns the most specific rule covering a file, mirroring how the reference
// implementation (scanoss.py) resolves the same settings.
//
// Picking the most specific rule rather than the first one that fits is not a refinement here, it
// is the whole contract: two rules can cover one file and name different replacements, and which
// of them wins decides what the report says. "Whichever appears first in the JSON" would make the
// answer depend on how the file was edited.
func bestReplaceRule(filePath string, purls []string, rules []settings.BOMEntry) (settings.BOMEntry, bool) {
	var best settings.BOMEntry
	bestScore, bestPathLen, found := -1, -1, false

	for _, rule := range rules {
		if rule.ReplaceWith == "" {
			continue
		}
		score, ok := ruleScore(rule, filePath, purls)
		if !ok {
			continue
		}
		// A longer path breaks a tie between rules of equal specificity: "vendor/lib/x.c" is a
		// narrower claim than "vendor/", so it is the one that meant this file.
		if pathLen := len(rule.Path); score > bestScore || (score == bestScore && pathLen > bestPathLen) {
			best, bestScore, bestPathLen, found = rule, score, pathLen, true
		}
	}
	return best, found
}

// ruleScore reports how specific a rule is about a file, and whether it covers it at all: 4 for
// both PURL and path, 2 for a PURL alone, 1 for a path alone. A rule stating neither covers
// everything and so identifies nothing; it is ignored rather than applied to the whole tree.
func ruleScore(rule settings.BOMEntry, filePath string, purls []string) (int, bool) {
	hasPurl, hasPath := rule.Purl != "", rule.Path != ""
	switch {
	case !hasPurl && !hasPath:
		return 0, false
	case hasPath && !matchPath(rule.Path, filePath):
		return 0, false
	case hasPurl && !purlsContain(purls, rule.Purl):
		return 0, false
	case hasPurl && hasPath:
		return 4, true
	case hasPurl:
		return 2, true
	default:
		return 1, true
	}
}

// purlsContain reports whether want names any of purls, comparing without versions so a rule
// written against a specific release still covers the component.
func purlsContain(purls []string, want string) bool {
	want = stripVersion(want)
	for _, purl := range purls {
		if stripVersion(purl) == want {
			return true
		}
	}
	return false
}

// componentForPurl resolves a replacement PURL to a catalog entry and returns its url_hash.
//
// An entry the scan already carries is reused, so the replacement inherits what the KB knows about
// that component instead of a thinner copy of it. Failing that the entry is synthesised from the
// PURL alone — all that is known at this point — and keyed by the PURL itself, so every file
// replaced with it lands on one entry rather than a catalog of duplicates.
func componentForPurl(result *scanossapi.ScanResult, purl string) string {
	bare, _ := splitPurlVersion(purl)

	// Sorted, because map iteration order is random and two entries can carry the same PURL: the
	// same result must always resolve to the same one.
	hashes := make([]string, 0, len(result.Components))
	for hash := range result.Components {
		hashes = append(hashes, hash)
	}
	sort.Strings(hashes)
	for _, hash := range hashes {
		for _, p := range result.Components[hash].Purls {
			if stripVersion(p) == bare {
				return hash
			}
		}
	}

	if result.Components == nil {
		result.Components = make(map[string]scanossapi.ComponentResult)
	}
	result.Components[purl] = componentFromPurl(purl)
	return purl
}

// componentFromPurl builds a catalog entry out of a PURL and nothing else. The fields a scan would
// normally supply — url, release date, rank — are left empty rather than guessed: the PURL does
// not state them, and a plausible invention is worse than an honest gap.
func componentFromPurl(purl string) scanossapi.ComponentResult {
	bare, version := splitPurlVersion(purl)
	name, vendor := purlNameAndVendor(bare)
	return scanossapi.ComponentResult{
		Component: name,
		Vendor:    vendor,
		Purls:     []string{bare},
		Version:   version,
	}
}

// purlNameAndVendor splits a versionless PURL into its component name and vendor:
// "pkg:golang/github.com/spf13/cobra" yields "cobra" and "github.com/spf13". A PURL with no
// namespace ("pkg:npm/lodash") yields an empty vendor.
func purlNameAndVendor(purl string) (name, vendor string) {
	rest := strings.TrimPrefix(purl, "pkg:")
	slash := strings.Index(rest, "/")
	if slash < 0 {
		return rest, "" // no type separator: nothing to split on
	}
	rest = rest[slash+1:] // drop the PURL type ("npm", "golang", ...)
	if last := strings.LastIndex(rest, "/"); last >= 0 {
		return rest[last+1:], rest[:last]
	}
	return rest, ""
}

// pruneUnreferencedComponents drops catalog entries no file matches any more. Both BOM rules leave
// them behind — remove by clearing a file's matches, replace by pointing them elsewhere — and a
// component nothing refers to would otherwise be reported as found.
func pruneUnreferencedComponents(result *scanossapi.ScanResult) {
	referenced := make(map[string]bool)
	for _, f := range result.Files {
		for _, m := range f.Matches {
			referenced[m.UrlHash] = true
		}
	}
	for hash := range result.Components {
		if !referenced[hash] {
			delete(result.Components, hash)
		}
	}
}
