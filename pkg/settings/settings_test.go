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
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	// Create a temporary settings file
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "scanoss.json")
	content := `{
		"bom": {
			"identify": [
				{"purl": "pkg:npm/lodash@4.17.21"},
				{"purl": "pkg:npm/express@*"}
			],
			"ignore": [
				{"purl": "pkg:npm/debug@*"}
			],
			"remove": [
				{"purl": "pkg:maven/org.apache.commons/commons-lang3@3.12.0"}
			]
		}
	}`
	if err := os.WriteFile(settingsPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test settings file: %v", err)
	}

	settings, err := Load(settingsPath)
	if err != nil {
		t.Fatalf("Failed to load settings: %v", err)
	}

	if len(settings.BOM.Identify) != 2 {
		t.Errorf("Expected 2 identify entries, got %d", len(settings.BOM.Identify))
	}
	if settings.BOM.Identify[0].Purl != "pkg:npm/lodash@4.17.21" {
		t.Errorf("Expected purl 'pkg:npm/lodash@4.17.21', got '%s'", settings.BOM.Identify[0].Purl)
	}
	if settings.BOM.Identify[1].Purl != "pkg:npm/express@*" {
		t.Errorf("Expected purl 'pkg:npm/express@*', got '%s'", settings.BOM.Identify[1].Purl)
	}

	if len(settings.BOM.Ignore) != 1 {
		t.Errorf("Expected 1 ignore entry, got %d", len(settings.BOM.Ignore))
	}
	if settings.BOM.Ignore[0].Purl != "pkg:npm/debug@*" {
		t.Errorf("Expected purl 'pkg:npm/debug@*', got '%s'", settings.BOM.Ignore[0].Purl)
	}

	if len(settings.BOM.Remove) != 1 {
		t.Errorf("Expected 1 remove entry, got %d", len(settings.BOM.Remove))
	}
	if settings.BOM.Remove[0].Purl != "pkg:maven/org.apache.commons/commons-lang3@3.12.0" {
		t.Errorf("Expected purl 'pkg:maven/org.apache.commons/commons-lang3@3.12.0', got '%s'", settings.BOM.Remove[0].Purl)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "scanoss.json")
	if err := os.WriteFile(settingsPath, []byte("not json"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err := Load(settingsPath)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	_, err := Load("/nonexistent/path/scanoss.json")
	if err == nil {
		t.Error("Expected error for non-existent file, got nil")
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		wantFind bool
	}{
		{"scanoss.json", "scanoss.json", true},
		{"settings.json", "settings.json", true},
		{"no settings file", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.fileName != "" {
				content := `{"bom": {}}`
				if err := os.WriteFile(filepath.Join(dir, tt.fileName), []byte(content), 0644); err != nil {
					t.Fatalf("Failed to write test file: %v", err)
				}
			}

			result := Detect(dir)
			if tt.wantFind && result == "" {
				t.Error("Expected to find settings file, got empty string")
			}
			if !tt.wantFind && result != "" {
				t.Errorf("Expected no settings file, got '%s'", result)
			}
			if tt.wantFind && result != "" {
				expectedPath := filepath.Join(dir, tt.fileName)
				if result != expectedPath {
					t.Errorf("Expected path '%s', got '%s'", expectedPath, result)
				}
			}
		})
	}
}

func TestDetectPriority(t *testing.T) {
	// When both files exist, scanoss.json should take priority
	dir := t.TempDir()
	content := `{"bom": {}}`
	os.WriteFile(filepath.Join(dir, "scanoss.json"), []byte(content), 0644)
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(content), 0644)

	result := Detect(dir)
	expected := filepath.Join(dir, "scanoss.json")
	if result != expected {
		t.Errorf("Expected scanoss.json to have priority, got '%s'", result)
	}
}

func TestDetectWithFilePath(t *testing.T) {
	// When Detect is called with a file path, it should check the parent directory
	dir := t.TempDir()
	content := `{"bom": {}}`
	os.WriteFile(filepath.Join(dir, "scanoss.json"), []byte(content), 0644)

	// Create a dummy file in the directory
	dummyFile := filepath.Join(dir, "main.go")
	os.WriteFile(dummyFile, []byte("package main"), 0644)

	result := Detect(dummyFile)
	expected := filepath.Join(dir, "scanoss.json")
	if result != expected {
		t.Errorf("Expected to find settings in parent dir, got '%s'", result)
	}
}

func TestHasBOM(t *testing.T) {
	tests := []struct {
		name     string
		settings Settings
		expected bool
	}{
		{"empty BOM", Settings{}, false},
		{"with identify", Settings{BOM: BOM{Identify: []BOMEntry{{Purl: "pkg:npm/test@1.0.0"}}}}, true},
		{"with include", Settings{BOM: BOM{Include: []BOMEntry{{Purl: "pkg:npm/test@1.0.0"}}}}, true},
		{"with ignore", Settings{BOM: BOM{Ignore: []BOMEntry{{Purl: "pkg:npm/test@1.0.0"}}}}, true},
		{"with exclude", Settings{BOM: BOM{Exclude: []BOMEntry{{Purl: "pkg:npm/test@1.0.0"}}}}, true},
		{"with remove", Settings{BOM: BOM{Remove: []BOMEntry{{Purl: "pkg:npm/test@1.0.0"}}}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.settings.HasBOM()
			if result != tt.expected {
				t.Errorf("Expected HasBOM() = %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestLoadSkip(t *testing.T) {
	dir := t.TempDir()
	content := `{
		"settings": {
			"skip": {
				"patterns": {
					"scanning": ["*.min.js", "dist/**"],
					"fingerprinting": ["**/*.ts"]
				},
				"sizes": {
					"scanning": [{ "patterns": ["*.bin"], "min": 0, "max": 1024 }]
				}
			}
		}
	}`
	path := filepath.Join(dir, "scanoss.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	patterns := s.Settings.SkipPatterns(OperationScanning)
	if len(patterns) != 2 || patterns[0] != "*.min.js" || patterns[1] != "dist/**" {
		t.Fatalf("SkipPatterns(scanning) = %v, want [*.min.js dist/**]", patterns)
	}
	if fp := s.Settings.SkipPatterns(OperationFingerprinting); len(fp) != 1 || fp[0] != "**/*.ts" {
		t.Fatalf("SkipPatterns(fingerprinting) = %v, want [**/*.ts]", fp)
	}
	sizes := s.Settings.SkipSizes(OperationScanning)
	if len(sizes) != 1 || sizes[0].Max != 1024 || len(sizes[0].Patterns) != 1 {
		t.Fatalf("SkipSizes(scanning) = %+v, want one rule with max 1024", sizes)
	}
}

func TestLoadFileSnippet(t *testing.T) {
	dir := t.TempDir()
	content := `{
		"settings": {
			"file_snippet": {
				"skip_headers": true,
				"skip_headers_limit": 25
			}
		}
	}`
	path := filepath.Join(dir, "scanoss.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if v, ok := s.Settings.SkipHeaders(); !ok || !v {
		t.Errorf("SkipHeaders() = (%v, %v), want (true, true)", v, ok)
	}
	if v, ok := s.Settings.SkipHeadersLimit(); !ok || v != 25 {
		t.Errorf("SkipHeadersLimit() = (%v, %v), want (25, true)", v, ok)
	}
}

// A settings file that says nothing about the file_snippet knobs must be distinguishable from
// one that sets them to their zero values: it is what decides whether the file overrides the
// command line.
func TestFileSnippetUnsetIsNotFalse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scanoss.json")
	if err := os.WriteFile(path, []byte(`{"bom": {}}`), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if v, ok := s.Settings.SkipHeaders(); ok {
		t.Errorf("SkipHeaders() = (%v, %v), want ok=false for an absent section", v, ok)
	}
	if v, ok := s.Settings.SkipHeadersLimit(); ok {
		t.Errorf("SkipHeadersLimit() = (%v, %v), want ok=false for an absent section", v, ok)
	}
}

func TestFileSnippetExplicitZeroIsSet(t *testing.T) {
	dir := t.TempDir()
	content := `{"settings": {"file_snippet": {"skip_headers": false, "skip_headers_limit": 0}}}`
	path := filepath.Join(dir, "scanoss.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if v, ok := s.Settings.SkipHeaders(); !ok || v {
		t.Errorf("SkipHeaders() = (%v, %v), want (false, true): an explicit false is a decision", v, ok)
	}
	if v, ok := s.Settings.SkipHeadersLimit(); !ok || v != 0 {
		t.Errorf("SkipHeadersLimit() = (%v, %v), want (0, true): 0 means 'no cap', not 'unset'", v, ok)
	}
}

// A negative cap would ask to drop fewer than zero lines. It is reported as unset rather than
// passed on to the fingerprinter.
func TestSkipHeadersLimitRejectsNegative(t *testing.T) {
	limit := -5
	tuning := Tuning{FileSnippet: FileSnippet{SkipHeadersLimit: &limit}}
	if v, ok := tuning.SkipHeadersLimit(); ok {
		t.Errorf("SkipHeadersLimit() = (%v, %v), want ok=false for a negative cap", v, ok)
	}
}

// bom.identify/bom.include and bom.ignore/bom.exclude each spell one rule. Both spellings must
// reach the consumer, whichever the settings file happens to use.
func TestRuleSpellingsAreFolded(t *testing.T) {
	tests := []struct {
		name  string
		bom   BOM
		rules func(BOM) []BOMEntry
		want  []string
	}{
		{
			"identify only",
			BOM{Identify: []BOMEntry{{Purl: "pkg:npm/a"}}},
			BOM.IdentifyRules, []string{"pkg:npm/a"},
		},
		{
			"include only",
			BOM{Include: []BOMEntry{{Purl: "pkg:npm/b"}}},
			BOM.IdentifyRules, []string{"pkg:npm/b"},
		},
		{
			"identify and include joined",
			BOM{Identify: []BOMEntry{{Purl: "pkg:npm/a"}}, Include: []BOMEntry{{Purl: "pkg:npm/b"}}},
			BOM.IdentifyRules, []string{"pkg:npm/a", "pkg:npm/b"},
		},
		{"no identify rules", BOM{}, BOM.IdentifyRules, nil},
		{
			"ignore only",
			BOM{Ignore: []BOMEntry{{Purl: "pkg:npm/c"}}},
			BOM.IgnoreRules, []string{"pkg:npm/c"},
		},
		{
			"exclude only",
			BOM{Exclude: []BOMEntry{{Purl: "pkg:npm/d"}}},
			BOM.IgnoreRules, []string{"pkg:npm/d"},
		},
		{
			"ignore and exclude joined",
			BOM{Ignore: []BOMEntry{{Purl: "pkg:npm/c"}}, Exclude: []BOMEntry{{Purl: "pkg:npm/d"}}},
			BOM.IgnoreRules, []string{"pkg:npm/c", "pkg:npm/d"},
		},
		{"no ignore rules", BOM{}, BOM.IgnoreRules, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.rules(tt.bom)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d rules, want %d: %+v", len(got), len(tt.want), got)
			}
			for i, purl := range tt.want {
				if got[i].Purl != purl {
					t.Errorf("rule %d = %q, want %q", i, got[i].Purl, purl)
				}
			}
		})
	}
}

// Folding must copy rather than extend one of the source slices: appending to a slice with spare
// capacity would write into whatever the settings file's own list shares memory with.
func TestJoinRulesDoesNotAliasItsInputs(t *testing.T) {
	identify := make([]BOMEntry, 1, 4) // spare capacity: append would write in place
	identify[0] = BOMEntry{Purl: "pkg:npm/a"}
	bom := BOM{Identify: identify, Include: []BOMEntry{{Purl: "pkg:npm/b"}}}

	joined := bom.IdentifyRules()
	joined[1].Purl = "pkg:npm/mutated"

	if bom.Identify[0].Purl != "pkg:npm/a" {
		t.Errorf("identify rule mutated to %q", bom.Identify[0].Purl)
	}
	if bom.Include[0].Purl != "pkg:npm/b" {
		t.Errorf("include rule mutated to %q", bom.Include[0].Purl)
	}
}
