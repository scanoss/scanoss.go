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
	scanossapi "github.com/scanoss/scanoss.api-sdk"
	"path/filepath"
	"strings"

	"github.com/scanoss/scanoss.go/pkg/settings"
)

// ApplyBOMRemove applies bom.remove rules to a scan result in place. For each file whose
// matched PURL matches a remove rule (and path), the match is neutralized (match_type
// "none", matches cleared), preserving the file path and hash. A file's PURLs are
// resolved by joining each match's url_hash to the component catalog. bom.include PURLs
// are protected. Components no longer referenced by any file are pruned from the catalog.
// A nil result or nil/empty rules is a no-op.
func ApplyBOMRemove(result *scanossapi.ScanResult, bom *settings.BOM) {
	if result == nil || bom == nil || len(bom.Remove) == 0 {
		return
	}

	includedPurls := make(map[string]bool)
	for _, entry := range bom.Include {
		includedPurls[stripVersion(entry.Purl)] = true
	}

	// If any of a file's matched PURLs matches a remove rule, the whole match is
	// neutralized to "none" — a partial match is not trustworthy. bom.include
	// PURLs are protected.
	for i := range result.Files {
		f := &result.Files[i]
		if f.MatchType == "" || f.MatchType == "none" {
			continue
		}
		if !shouldRemove(f.Path, filePurls(f, result.Components), bom.Remove, includedPurls) {
			continue
		}
		f.MatchType = "none"
		f.Matches = nil
	}

	pruneUnreferencedComponents(result)
}

// filePurls gathers the PURLs a file matched, by joining each match's url_hash to the
// component catalog.
func filePurls(f *scanossapi.FileResult, components map[string]scanossapi.ComponentResult) []string {
	var purls []string
	for _, m := range f.Matches {
		purls = append(purls, components[m.UrlHash].Purls...)
	}
	return purls
}

// shouldRemove checks whether a file's matches should be removed based on bom.remove
// rules. Returns false if any of the file's PURLs are protected by bom.include.
func shouldRemove(filePath string, entryPurls []string, removeRules []settings.BOMEntry, includedPurls map[string]bool) bool {
	for _, purl := range entryPurls {
		normalizedPurl := stripVersion(purl)

		// bom.include takes precedence: if this PURL is included, don't remove
		if includedPurls[normalizedPurl] {
			return false
		}
	}

	for _, rule := range removeRules {
		ruleNormalized := stripVersion(rule.Purl)

		for _, purl := range entryPurls {
			entryNormalized := stripVersion(purl)

			if ruleNormalized != entryNormalized {
				continue
			}

			// PURL matches — now check path
			if rule.Path == "" {
				return true
			}

			if matchPath(rule.Path, filePath) {
				return true
			}
		}
	}
	return false
}

// stripVersion returns a PURL without its version component.
// e.g., "pkg:npm/lodash@4.17.21" -> "pkg:npm/lodash"
// If no version is present, returns the PURL as-is.
func stripVersion(purl string) string {
	bare, _ := splitPurlVersion(purl)
	return bare
}

// splitPurlVersion splits a PURL into its versionless form and its version.
//
// The version is the part after the last "@", but only when that "@" comes after the last "/":
// a scoped npm PURL carries one in its namespace ("pkg:npm/@babel/core"), and reading that as a
// version would leave "pkg:npm/" behind and quietly match every npm package.
func splitPurlVersion(purl string) (bare, version string) {
	at := strings.LastIndex(purl, "@")
	if at <= 0 || at < strings.LastIndex(purl, "/") {
		return purl, ""
	}
	return purl[:at], purl[at+1:]
}

// matchPath checks if a file path matches a bom.remove path pattern.
// Rules:
//   - "someFolder/" means recursive match (anything under that folder)
//   - "someFolder/*" means single-level match (direct children only)
//   - Standard glob patterns via filepath.Match
func matchPath(pattern, filePath string) bool {
	// Normalize separators
	pattern = filepath.ToSlash(pattern)
	filePath = filepath.ToSlash(filePath)

	// Recursive match: pattern ends with "/"
	if strings.HasSuffix(pattern, "/") {
		prefix := pattern // e.g., "someFolder/"
		return strings.HasPrefix(filePath, prefix) || filePath == strings.TrimSuffix(prefix, "/")
	}

	// Single-level glob: e.g., "someFolder/*"
	// Use filepath.Match for standard glob matching
	matched, err := filepath.Match(pattern, filePath)
	if err == nil && matched {
		return true
	}

	return false
}
