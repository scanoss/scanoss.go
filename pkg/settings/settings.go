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
