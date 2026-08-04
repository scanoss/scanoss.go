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

func TestResolve(t *testing.T) {
	t.Run("explicit settings flag", func(t *testing.T) {
		dir := t.TempDir()
		settingsPath := filepath.Join(dir, "my-settings.json")
		content := `{"bom": {"identify": [{"purl": "pkg:npm/test@1.0.0"}]}}`
		os.WriteFile(settingsPath, []byte(content), 0644)

		s, err := Resolve(settingsPath, dir)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if s == nil {
			t.Fatal("Expected settings, got nil")
		}
		if len(s.BOM.Identify) != 1 {
			t.Errorf("Expected 1 identify entry, got %d", len(s.BOM.Identify))
		}
	})

	t.Run("auto-detect", func(t *testing.T) {
		dir := t.TempDir()
		content := `{"bom": {"ignore": [{"purl": "pkg:npm/auto@*"}]}}`
		os.WriteFile(filepath.Join(dir, "scanoss.json"), []byte(content), 0644)

		s, err := Resolve("", dir)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if s == nil {
			t.Fatal("Expected settings, got nil")
		}
		if len(s.BOM.Ignore) != 1 {
			t.Errorf("Expected 1 ignore entry, got %d", len(s.BOM.Ignore))
		}
	})

	t.Run("no settings", func(t *testing.T) {
		dir := t.TempDir()

		s, err := Resolve("", dir)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if s != nil {
			t.Error("Expected nil settings, got non-nil")
		}
	})

	t.Run("explicit flag overrides auto-detect", func(t *testing.T) {
		dir := t.TempDir()
		// Auto-detect file in scan dir
		os.WriteFile(filepath.Join(dir, "scanoss.json"), []byte(`{"bom": {"identify": [{"purl": "pkg:npm/auto@1.0.0"}]}}`), 0644)

		// Explicit settings file elsewhere
		explicitDir := t.TempDir()
		explicitPath := filepath.Join(explicitDir, "custom.json")
		os.WriteFile(explicitPath, []byte(`{"bom": {"identify": [{"purl": "pkg:npm/explicit@2.0.0"}]}}`), 0644)

		s, err := Resolve(explicitPath, dir)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if s == nil {
			t.Fatal("Expected settings, got nil")
		}
		if s.BOM.Identify[0].Purl != "pkg:npm/explicit@2.0.0" {
			t.Errorf("Expected explicit settings to override, got purl '%s'", s.BOM.Identify[0].Purl)
		}
	})

	t.Run("non-existent explicit file", func(t *testing.T) {
		_, err := Resolve("/nonexistent/settings.json", t.TempDir())
		if err == nil {
			t.Error("Expected error for non-existent explicit file")
		}
	})
}

func TestHasBOM(t *testing.T) {
	tests := []struct {
		name     string
		settings Settings
		expected bool
	}{
		{"empty BOM", Settings{}, false},
		{"with identify", Settings{BOM: BOM{Identify: []BOMEntry{{Purl: "pkg:npm/test@1.0.0"}}}}, true},
		{"with ignore", Settings{BOM: BOM{Ignore: []BOMEntry{{Purl: "pkg:npm/test@1.0.0"}}}}, true},
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

// ScanFilter and FingerprintFilter must read their own operation's rules.
// scanoss.json keeps them apart, so a command that fingerprints (wfp) and one
// that scans must not pick up each other's patterns.
func TestScanAndFingerprintFiltersAreDistinct(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scanoss.json")
	content := `{
	  "settings": {
	    "skip": {
	      "patterns": {
	        "scanning":       ["only-scanning/**"],
	        "fingerprinting": ["only-fingerprinting/**"]
	      }
	    }
	  }
	}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	scan := s.ScanFilter()
	if got := scan.Skip.Patterns; len(got) != 1 || got[0] != "only-scanning/**" {
		t.Errorf("ScanFilter patterns = %v, want [only-scanning/**]", got)
	}
	fingerprint := s.FingerprintFilter()
	if got := fingerprint.Skip.Patterns; len(got) != 1 || got[0] != "only-fingerprinting/**" {
		t.Errorf("FingerprintFilter patterns = %v, want [only-fingerprinting/**]", got)
	}
}

func TestFingerprintFilterNil(t *testing.T) {
	var s *Settings
	if got := s.FingerprintFilter(); got != nil {
		t.Fatalf("FingerprintFilter() on nil = %v, want nil", got)
	}
}

// The three accessors must each read their own section. A command that copied
// another's call would silently apply the wrong project rules, which is exactly
// the kind of mistake no test would otherwise catch.
func TestFilterAccessorsReadTheirOwnSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scanoss.json")
	content := `{
	  "settings": {
	    "skip": {
	      "patterns": {
	        "scanning":       ["only-scanning/**"],
	        "fingerprinting": ["only-fingerprinting/**"],
	        "dependencies":   ["only-dependencies/**"]
	      }
	    }
	  }
	}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	for name, tc := range map[string]struct {
		got  []string
		want string
	}{
		"ScanFilter":        {s.ScanFilter().Skip.Patterns, "only-scanning/**"},
		"FingerprintFilter": {s.FingerprintFilter().Skip.Patterns, "only-fingerprinting/**"},
		"DependencyFilter":  {s.DependencyFilter().Skip.Patterns, "only-dependencies/**"},
	} {
		if len(tc.got) != 1 || tc.got[0] != tc.want {
			t.Errorf("%s patterns = %v, want [%s]", name, tc.got, tc.want)
		}
	}
}

func TestDependencyFilterNil(t *testing.T) {
	var s *Settings
	if got := s.DependencyFilter(); got != nil {
		t.Fatalf("DependencyFilter() on nil = %v, want nil", got)
	}
}
