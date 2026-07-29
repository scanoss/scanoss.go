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
	"strings"

	"github.com/scanoss/scanoss.go/pkg/filter"
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
type BOM struct {
	// Include specifies components that should be included in scan results (new format)
	Include []BOMEntry `json:"include,omitempty"`
	// Identify specifies components that should be identified as declared dependencies
	Identify []BOMEntry `json:"identify,omitempty"`
	// Ignore specifies components that should be ignored/whitelisted in scan results
	Ignore []BOMEntry `json:"ignore,omitempty"`
	// Remove specifies components that should be removed/blacklisted from scan results
	Remove []BOMEntry `json:"remove,omitempty"`
	// Replace specifies components that should be replaced with alternatives
	Replace []BOMEntry `json:"replace,omitempty"`
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

// Tuning mirrors the top-level settings section of scanoss.json: the
// input-filtering skip rules applied during file collection.
type Tuning struct {
	Skip Skip `json:"skip,omitempty"`
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

// filterFor maps the skip rules for the given operation (one of the Operation*
// constants) into the filter package's dependency-free Settings. Returns nil
// when s is nil.
func (s *Settings) filterFor(operation string) *filter.Settings {
	if s == nil {
		return nil
	}
	sizes := s.Settings.SkipSizes(operation)
	rules := make([]filter.SizeRule, 0, len(sizes))
	for _, r := range sizes {
		rules = append(rules, filter.SizeRule{Patterns: r.Patterns, Min: r.Min, Max: r.Max})
	}
	return &filter.Settings{
		Skip: filter.Skip{
			Patterns: s.Settings.SkipPatterns(operation),
			Sizes:    rules,
		},
	}
}

// ScanFilter returns the file-collection filter for the scanning operation,
// derived from the scanoss.json skip rules. Returns nil when s is nil.
func (s *Settings) ScanFilter() *filter.Settings {
	return s.filterFor(OperationScanning)
}

// FingerprintFilter returns the file-collection filter for the fingerprinting
// operation. scanoss.json keeps the two operations apart, so a command that only
// fingerprints (wfp) must read its own rules rather than the scanning ones.
// Returns nil when s is nil.
func (s *Settings) FingerprintFilter() *filter.Settings {
	return s.filterFor(OperationFingerprinting)
}

// DependencyFilter returns the file-collection filter for the dependencies
// operation. The section is part of the published scanoss.json schema, so a
// project can already write skip.patterns.dependencies today — this is what
// makes it take effect. Returns nil when s is nil.
func (s *Settings) DependencyFilter() *filter.Settings {
	return s.filterFor(OperationDependencies)
}

// HasBOM returns true if the settings contain any BOM entries
func (s *Settings) HasBOM() bool {
	return len(s.BOM.Include) > 0 ||
		len(s.BOM.Identify) > 0 ||
		len(s.BOM.Ignore) > 0 ||
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

// Detect looks for a settings file in the given directory.
// It checks for both "scanoss.json" and "settings.json" (in that order).
// Returns the path to the first found file, or empty string if none found.
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

// Resolve determines the settings file to use based on the provided flag value
// and the scan target path. The --settings flag takes highest priority;
// if not provided, auto-detection in the scan folder is attempted.
//
// Returns the loaded Settings (or nil if no settings file), and an error if loading fails.
func Resolve(settingsFlag string, scanPath string) (*Settings, error) {
	var settingsPath string

	if settingsFlag != "" {
		// --settings flag provided: resolve path
		if filepath.IsAbs(settingsFlag) {
			settingsPath = settingsFlag
		} else {
			// Relative to current working directory
			absPath, err := filepath.Abs(settingsFlag)
			if err != nil {
				return nil, fmt.Errorf("error resolving settings path: %w", err)
			}
			settingsPath = absPath
		}

		// Verify file exists
		if _, err := os.Stat(settingsPath); err != nil {
			return nil, fmt.Errorf("settings file not found: %s", settingsPath)
		}
	} else {
		// Auto-detect in scan folder
		settingsPath = Detect(scanPath)
		if settingsPath == "" {
			return nil, nil // No settings file found, not an error
		}
	}

	return Load(settingsPath)
}

// SBOMData represents SBOM data to be sent to the API
type SBOMData struct {
	Assets string // JSON-serialized SBOM: {"components": [{"purl": "pkg:..."}, ...]}
	Type   string // "identify" or "blacklist"
}

// GetSBOMData converts BOM entries into the format expected by SCANOSS API (compatible with scanoss.py)
// Returns nil if there are no BOM entries that should be sent to the API
// Note: Remove entries are NOT sent to the API - they are applied as post-processing
func GetSBOMData(bom *BOM) *SBOMData {
	if bom == nil {
		return nil
	}

	// Collect purls based on priority:
	// 1. Include + Replace → scan_type='identify'
	// 2. Identify → scan_type='identify'
	// 3. Ignore → scan_type='blacklist'
	// Note: Remove is NOT sent to API, it's applied as post-processing

	var purls []string
	var scanType string

	// New format: Include + Replace (highest priority)
	if len(bom.Include) > 0 || len(bom.Replace) > 0 {
		scanType = "identify"
		for _, entry := range bom.Include {
			purls = append(purls, entry.Purl)
		}
		for _, entry := range bom.Replace {
			purls = append(purls, entry.Purl)
		}
	} else if len(bom.Identify) > 0 {
		// Legacy format: Identify
		scanType = "identify"
		for _, entry := range bom.Identify {
			purls = append(purls, entry.Purl)
		}
	} else if len(bom.Ignore) > 0 {
		// Blacklist format: Ignore only
		scanType = "blacklist"
		for _, entry := range bom.Ignore {
			purls = append(purls, entry.Purl)
		}
	} else {
		// No entries to send to API (Remove is handled separately)
		return nil
	}

	// Convert purls to SBOM component objects (format expected by SCANOSS engine)
	// Each component must be an object with a "purl" field
	components := make([]map[string]string, 0, len(purls))
	for _, purl := range purls {
		components = append(components, map[string]string{
			"purl": purl,
		})
	}

	// Wrap components in SBOM structure (compatible with SCANOSS engine)
	assetsObject := map[string]interface{}{
		"components": components,
	}

	// Serialize to JSON
	assetsJSON, err := json.Marshal(assetsObject)
	if err != nil {
		// Fallback to empty components array if serialization fails
		assetsJSON = []byte(`{"components":[]}`)
	}

	return &SBOMData{
		Assets: string(assetsJSON),
		Type:   scanType,
	}
}

// FormatSBOMParam formats BOM entries into the SBOM parameter string
// Deprecated: Use GetSBOMData instead for compatibility with scanoss.py
// The format is: "type=identify\npurl1\npurl2\ntype=ignore\npurl3\n..."
func FormatSBOMParam(bom *BOM) string {
	if bom == nil {
		return ""
	}

	var parts []string

	// Include (new format)
	if len(bom.Include) > 0 {
		parts = append(parts, "type=identify")
		for _, entry := range bom.Include {
			parts = append(parts, entry.Purl)
		}
	}

	// Identify (legacy format)
	if len(bom.Identify) > 0 {
		if len(parts) == 0 {
			parts = append(parts, "type=identify")
		}
		for _, entry := range bom.Identify {
			parts = append(parts, entry.Purl)
		}
	}

	// Ignore
	if len(bom.Ignore) > 0 {
		parts = append(parts, "type=ignore")
		for _, entry := range bom.Ignore {
			parts = append(parts, entry.Purl)
		}
	}

	// Remove
	if len(bom.Remove) > 0 {
		parts = append(parts, "type=remove")
		for _, entry := range bom.Remove {
			parts = append(parts, entry.Purl)
		}
	}

	// Replace (new format)
	if len(bom.Replace) > 0 {
		parts = append(parts, "type=replace")
		for _, entry := range bom.Replace {
			parts = append(parts, entry.Purl)
		}
	}

	return strings.Join(parts, "\n")
}
