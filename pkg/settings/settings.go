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

package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Settings file names supported (both conventions)
var settingsFileNames = []string{"scanoss.json", "settings.json"}

// BOMEntry represents a single BOM (Bill of Materials) entry
// that specifies how the scanner should handle a particular component.
type BOMEntry struct {
	// PURL (Package URL) identifying the component. Supports glob patterns (e.g., pkg:npm/lodash@*)
	Purl string `json:"purl"`
	// Path optional file path pattern to scope this entry
	Path string `json:"path,omitempty"`
	// ReplaceWith specifies the PURL to replace with (only for Replace entries)
	ReplaceWith string `json:"replace_with,omitempty"`
}

// BOM represents the Bill of Materials section in the settings file.
// It contains lists of components to identify, ignore, or remove from scan results.
//
// Two of the rules are spelled two ways, because the published scanoss.json schema and this
// client's own settings grew apart: the schema calls them "include" and "exclude", while this
// client declared "identify" and "ignore". Both spellings are read, and are folded together by
// IdentifyRules and IgnoreRules — which is what consumers should use rather than the fields.
type BOM struct {
	// Include specifies components declared to be present. Schema spelling of Identify.
	Include []BOMEntry `json:"include,omitempty"`
	// Identify specifies components that should be identified as declared dependencies
	Identify []BOMEntry `json:"identify,omitempty"`
	// Ignore specifies components that should be ignored/whitelisted in scan results
	Ignore []BOMEntry `json:"ignore,omitempty"`
	// Exclude specifies components to drop from matches. Schema spelling of Ignore.
	Exclude []BOMEntry `json:"exclude,omitempty"`
	// Remove specifies components that should be removed/blacklisted from scan results
	Remove []BOMEntry `json:"remove,omitempty"`
	// Replace specifies components that should be replaced with alternatives
	Replace []BOMEntry `json:"replace,omitempty"`
}

// IdentifyRules returns the components the user declares are present: bom.identify and
// bom.include together, since the two spell one rule.
//
// The order the two lists are joined in is not a precedence mechanism. Where several rules cover
// one file, the most specific one wins (see the rule scoring in the postprocess package), so a
// caller must not read "earlier in this slice" as "stronger".
func (b BOM) IdentifyRules() []BOMEntry { return joinRules(b.Identify, b.Include) }

// IgnoreRules returns the components to drop from a file's matches: bom.ignore and bom.exclude
// together, since the two spell one rule. The ordering caveat on IdentifyRules applies here too.
func (b BOM) IgnoreRules() []BOMEntry { return joinRules(b.Ignore, b.Exclude) }

// joinRules concatenates two rule lists, returning the other one untouched when either is empty —
// the common case, since a settings file uses one spelling or the other, not both.
func joinRules(a, b []BOMEntry) []BOMEntry {
	switch {
	case len(a) == 0:
		return b
	case len(b) == 0:
		return a
	}
	out := make([]BOMEntry, 0, len(a)+len(b))
	out = append(out, a...)
	return append(out, b...)
}

// Operation identifies which set of skip rules applies. Mirrors the operations
// enumerated in the scanoss.json settings schema.
const (
	OperationScanning       = "scanning"
	OperationFingerprinting = "fingerprinting"
	OperationDependencies   = "dependencies"
)

// SizeRule is one entry under settings.skip.sizes.<operation>: files matching any
// of Patterns are skipped when smaller than Min or larger than Max (0 disables a
// bound).
type SizeRule struct {
	Patterns []string `json:"patterns"`
	Min      int64    `json:"min"`
	Max      int64    `json:"max"`
}

// SkipPatternsByOp mirrors settings.skip.patterns: glob patterns per operation.
type SkipPatternsByOp struct {
	Scanning       []string `json:"scanning,omitempty"`
	Fingerprinting []string `json:"fingerprinting,omitempty"`
	Dependencies   []string `json:"dependencies,omitempty"`
}

// SkipSizesByOp mirrors settings.skip.sizes: size rules per operation.
type SkipSizesByOp struct {
	Scanning       []SizeRule `json:"scanning,omitempty"`
	Fingerprinting []SizeRule `json:"fingerprinting,omitempty"`
	Dependencies   []SizeRule `json:"dependencies,omitempty"`
}

// Skip mirrors the settings.skip section of scanoss.json.
type Skip struct {
	Patterns SkipPatternsByOp `json:"patterns,omitempty"`
	Sizes    SkipSizesByOp    `json:"sizes,omitempty"`
}

// FileSnippet mirrors settings.file_snippet: the knobs that tune how files are fingerprinted
// and matched.
//
// Only the ones this client acts on are declared. The rest of the section
// (min_snippet_hits, min_snippet_lines, ranking_enabled, honour_file_exts) tunes the matching
// engine, not the client, and this client does not forward scan settings to the server — so
// declaring them here would promise something nothing honors.
//
// ranking_threshold is the exception that proves the rule: scanoss.py sends it to the server,
// which filters before reporting. Here it is applied to the result the batch scanner returns,
// which reports every candidate match with its rank — so the same setting reaches the same
// outcome without the server needing to know about it.
//
// The fields are pointers because the schema's "unset" is a value, not an absence: skip_headers
// defaults to false, skip_headers_limit to 0 and ranking_threshold to 0, so a plain bool/int
// cannot say whether the settings file asked for that or said nothing at all. That difference is
// what decides whether the file overrides the command line.
type FileSnippet struct {
	// SkipHeaders skips license headers, comments and imports at the start of each file
	// when fingerprinting.
	SkipHeaders *bool `json:"skip_headers,omitempty"`
	// SkipHeadersLimit caps how many leading lines SkipHeaders may drop (0 = no cap).
	SkipHeadersLimit *int `json:"skip_headers_limit,omitempty"`
	// RankingThreshold drops matches whose component ranks worse than this. Rank is the
	// scanner's own ordering, lowest is strongest. Valid range -1..MaxRankingThreshold;
	// anything at or below 0 disables the filter.
	RankingThreshold *int `json:"ranking_threshold,omitempty"`
}

// MaxRankingThreshold is the largest ranking threshold the settings schema accepts. Ranks
// observed in practice run 1..9, so a higher cap would filter nothing that 10 does not.
const MaxRankingThreshold = 10

// MinRankingThreshold is the smallest accepted value: -1 means "defer", which for a client-side
// filter is the same as off.
const MinRankingThreshold = -1

// Tuning mirrors the top-level settings section of scanoss.json: the input-filtering skip rules
// applied during file collection, and the file_snippet fingerprinting knobs.
type Tuning struct {
	Skip        Skip        `json:"skip,omitempty"`
	FileSnippet FileSnippet `json:"file_snippet,omitempty"`
}

// SkipHeaders reports the configured skip_headers value, and whether the settings file set it
// at all. An unset value is not "false": see the FileSnippet doc.
func (t Tuning) SkipHeaders() (value, ok bool) {
	if t.FileSnippet.SkipHeaders == nil {
		return false, false
	}
	return *t.FileSnippet.SkipHeaders, true
}

// SkipHeadersLimit reports the configured skip_headers_limit, and whether the settings file set
// it at all. A negative cap is meaningless — it would ask to drop fewer than zero lines — so it
// is reported as unset rather than passed on.
func (t Tuning) SkipHeadersLimit() (value int, ok bool) {
	if t.FileSnippet.SkipHeadersLimit == nil || *t.FileSnippet.SkipHeadersLimit < 0 {
		return 0, false
	}
	return *t.FileSnippet.SkipHeadersLimit, true
}

// RankingThreshold reports the configured ranking_threshold, and whether the settings file set it
// at all. The value is returned as written, out-of-range included: clamping it is a decision that
// owes the user a warning, which belongs where there is somewhere to print one.
func (t Tuning) RankingThreshold() (value int, ok bool) {
	if t.FileSnippet.RankingThreshold == nil {
		return 0, false
	}
	return *t.FileSnippet.RankingThreshold, true
}

// SkipPatterns returns the skip patterns for the given operation, or nil.
func (t Tuning) SkipPatterns(operation string) []string {
	switch operation {
	case OperationScanning:
		return t.Skip.Patterns.Scanning
	case OperationFingerprinting:
		return t.Skip.Patterns.Fingerprinting
	case OperationDependencies:
		return t.Skip.Patterns.Dependencies
	}
	return nil
}

// SkipSizes returns the size rules for the given operation, or nil.
func (t Tuning) SkipSizes(operation string) []SizeRule {
	switch operation {
	case OperationScanning:
		return t.Skip.Sizes.Scanning
	case OperationFingerprinting:
		return t.Skip.Sizes.Fingerprinting
	case OperationDependencies:
		return t.Skip.Sizes.Dependencies
	}
	return nil
}

// Settings represents the scanoss settings file structure.
type Settings struct {
	BOM BOM `json:"bom"`
	// Settings holds the input-filtering rules (the scanoss.json "settings"
	// section). Optional.
	Settings Tuning `json:"settings,omitempty"`
}

// HasBOM returns true if the settings contain any BOM entries
func (s *Settings) HasBOM() bool {
	return len(s.BOM.Include) > 0 ||
		len(s.BOM.Identify) > 0 ||
		len(s.BOM.Ignore) > 0 ||
		len(s.BOM.Exclude) > 0 ||
		len(s.BOM.Remove) > 0 ||
		len(s.BOM.Replace) > 0
}

// Load reads and parses a settings file from the given path.
func Load(path string) (*Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading settings file: %w", err)
	}

	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("error parsing settings file: %w", err)
	}

	return &settings, nil
}

// Detect looks for a settings file in the given directory, checking "scanoss.json" then
// "settings.json". It returns the path of the first one found, or "" if there is none —
// a project without settings is the normal case, not a failure.
//
// A file path is accepted too, and its directory is searched: a scan target can be a single
// file, and the settings that apply to it live alongside it.
func Detect(dir string) string {
	// Ensure we're checking a directory
	info, err := os.Stat(dir)
	if err != nil {
		return ""
	}
	// If dir is a file, use its parent directory
	if !info.IsDir() {
		dir = filepath.Dir(dir)
	}

	for _, name := range settingsFileNames {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}
