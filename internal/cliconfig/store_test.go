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

package cliconfig

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withHome points the package at a temporary home and clears the settings
// environment variables for the duration of one test.
func withHome(t *testing.T) string {
	t.Helper()
	isolateEnv(t)
	home := t.TempDir()
	original := homeDir
	homeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { homeDir = original })
	return home
}

// isolateEnv unsets every SCANOSS_* settings variable for the duration of one test.
// Without this, a developer (or a CI job) with SCANOSS_API_KEY exported would see
// their own credential win the precedence ladder and the tests would fail — or
// worse, pass for the wrong reason. t.Setenv records the original value so cleanup
// restores it; the Unsetenv afterwards is what makes the variable genuinely absent
// rather than empty.
func isolateEnv(t *testing.T) {
	t.Helper()
	for _, key := range recognizedKeys() {
		name := EnvName(key)
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("unsetting %s: %v", name, err)
		}
	}
}

// readFileMap returns the settings file parsed as a map, failing the test if it
// cannot be read.
func readFileMap(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return out
}

func TestPath(t *testing.T) {
	home := withHome(t)

	got, err := Path()
	if err != nil {
		t.Fatalf("Path() error: %v", err)
	}
	want := filepath.Join(home, ".scanoss", "settings.json")
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

// A missing file is an empty config, and reading must not create anything: a
// machine that never runs `config set` never gets a ~/.scanoss directory.
func TestLoadMissingFileCreatesNothing(t *testing.T) {
	home := withHome(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if keys := cfg.Keys(); len(keys) != 0 {
		t.Errorf("Keys() = %v, want empty", keys)
	}
	if _, ok := cfg.Get(KeyAPIKey); ok {
		t.Error("Get(api_key) reported set on a missing file")
	}
	if _, err := os.Stat(filepath.Join(home, ".scanoss")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("reading created %s (err = %v), want it left absent", filepath.Join(home, ".scanoss"), err)
	}
}

func TestLoadMalformedFile(t *testing.T) {
	home := withHome(t)
	dir := filepath.Join(home, ".scanoss")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"api_key": }`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("Load() succeeded on a malformed file, want an error")
	}
	// The message must name the file: the user has to know what to fix.
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not name %q", err, path)
	}
}

func TestGetTreatsEmptyValueAsUnset(t *testing.T) {
	withHome(t)
	if err := Set(KeyAPIKey, ""); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if value, ok := cfg.Get(KeyAPIKey); ok {
		t.Errorf("Get(api_key) = (%q, true), want unset for an empty value", value)
	}
}

// A hand-edited numeric value must still read back as a string rather than
// Go's %!s(float64=...) rendering.
func TestGetRendersNonStringValue(t *testing.T) {
	home := withHome(t)
	dir := filepath.Join(home, ".scanoss")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := `{"api_url": "https://api.scanoss.com", "some_threshold": 5}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	value, ok := cfg.Get("some_threshold")
	if !ok {
		t.Fatal("Get(some_threshold) reported unset")
	}
	if value != "5" {
		t.Errorf("Get(some_threshold) = %q, want %q", value, "5")
	}
}

// The first `config set` on a clean machine must succeed, creating both the
// directory and the file with credential-safe permissions.
func TestSetCreatesDirectoryAndFile(t *testing.T) {
	home := withHome(t)

	if err := Set(KeyAPIKey, "SC_abc123"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	dir := filepath.Join(home, ".scanoss")
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat %s: %v", dir, err)
	}
	if got := dirInfo.Mode().Perm(); got != dirPerm {
		t.Errorf("directory mode = %O, want %O", got, dirPerm)
	}

	path := filepath.Join(dir, "settings.json")
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := fileInfo.Mode().Perm(); got != filePerm {
		t.Errorf("file mode = %O, want %O", got, filePerm)
	}
	if got := readFileMap(t, path)[KeyAPIKey]; got != "SC_abc123" {
		t.Errorf("stored api_key = %v, want %q", got, "SC_abc123")
	}
}

// No temporary files may survive a successful write.
func TestSetLeavesNoTempFiles(t *testing.T) {
	home := withHome(t)
	if err := Set(KeyAPIURL, "https://api.scanoss.com"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(home, ".scanoss"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "settings.json" {
			t.Errorf("leftover file %q in the settings directory", entry.Name())
		}
	}
}

// An existing directory keeps whatever permissions the user gave it: tightening
// something the user created is not this command's decision.
func TestSetLeavesExistingDirectoryPermissions(t *testing.T) {
	home := withHome(t)
	dir := filepath.Join(home, ".scanoss")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Set(KeyAPIURL, "https://api.scanoss.com"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Errorf("directory mode = %O, want it left at %O", got, 0o755)
	}
}

func TestSetWhenSettingsDirectoryIsAFile(t *testing.T) {
	home := withHome(t)
	dir := filepath.Join(home, ".scanoss")
	if err := os.WriteFile(dir, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := Set(KeyAPIKey, "SC_abc123")
	if err == nil {
		t.Fatal("Set() succeeded with ~/.scanoss as a file, want an error")
	}
	if !strings.Contains(err.Error(), dir) {
		t.Errorf("error %q does not name %q", err, dir)
	}
	// Nothing may be written: the blocking path must be reported, not worked around.
	data, readErr := os.ReadFile(dir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "not a directory" {
		t.Errorf("the blocking file was modified: %q", data)
	}
}

// Keys this version does not recognize belong to the user; a write must not drop
// them, so hand-editing the file stays safe.
func TestSetPreservesUnrecognizedKeys(t *testing.T) {
	home := withHome(t)
	dir := filepath.Join(home, ".scanoss")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.json")
	body := `{"api_key": "SC_original", "another_setting": "keep me"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Set(KeyAPIURL, "https://api.scanoss.com"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	stored := readFileMap(t, path)
	for key, want := range map[string]any{
		KeyAPIKey:         "SC_original",
		KeyAPIURL:         "https://api.scanoss.com",
		"another_setting": "keep me",
	} {
		if got := stored[key]; got != want {
			t.Errorf("stored[%q] = %v, want %v", key, got, want)
		}
	}
}

// Keys are case-insensitive and normalized to lower case on write: viper lowercases
// every key it reads, so a hand-edited "MyCustom_Key" comes back as
// "mycustom_key". The value is preserved; only the spelling of the key changes.
// Documented here so the behaviour is a decision rather than a surprise.
func TestSetLowercasesKeys(t *testing.T) {
	home := withHome(t)
	dir := filepath.Join(home, ".scanoss")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"MyCustom_Key": "keep me"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Set(KeyAPIURL, "https://api.scanoss.com"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	stored := readFileMap(t, path)
	if got, ok := stored["mycustom_key"]; !ok || got != "keep me" {
		t.Errorf(`stored["mycustom_key"] = %v (present=%t), want %q`, got, ok, "keep me")
	}
	if _, ok := stored["MyCustom_Key"]; ok {
		t.Error("key case was preserved; this test documents that it is lowercased")
	}
}

func TestSetRejectsUnrecognizedKey(t *testing.T) {
	home := withHome(t)

	err := Set("api_token", "SC_abc123")
	var unknown *UnknownKeyError
	if !errors.As(err, &unknown) {
		t.Fatalf("Set() error = %v, want *UnknownKeyError", err)
	}
	if unknown.Key != "api_token" {
		t.Errorf("UnknownKeyError.Key = %q, want %q", unknown.Key, "api_token")
	}
	// The message must list what is valid, in the spelling the user types.
	for _, key := range []string{"api-key", "api-url"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q does not mention %q", err, key)
		}
	}
	// A rejected key must not create the file.
	if _, statErr := os.Stat(filepath.Join(home, ".scanoss")); !errors.Is(statErr, os.ErrNotExist) {
		t.Error("a rejected key created the settings directory")
	}
}

// FR-4a: a write must never persist a value that came from the environment.
// viper.WriteConfig would, because it serializes the merged view — this is the
// regression test for using it by mistake.
func TestSetDoesNotPersistEnvironmentValues(t *testing.T) {
	home := withHome(t)
	t.Setenv("SCANOSS_API_KEY", "ci_token_that_must_not_be_stored")

	if err := Set(KeyAPIURL, "https://api.scanoss.com"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	path := filepath.Join(home, ".scanoss", "settings.json")
	if _, ok := readFileMap(t, path)[KeyAPIKey]; ok {
		t.Error("api_key was written to the file from the environment")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "ci_token_that_must_not_be_stored") {
		t.Errorf("the environment value leaked into %s: %s", path, data)
	}
}

func TestUnset(t *testing.T) {
	home := withHome(t)
	if err := Set(KeyAPIKey, "SC_abc123"); err != nil {
		t.Fatal(err)
	}
	if err := Set(KeyAPIURL, "https://api.scanoss.com"); err != nil {
		t.Fatal(err)
	}

	if err := Unset(KeyAPIKey); err != nil {
		t.Fatalf("Unset() error: %v", err)
	}

	stored := readFileMap(t, filepath.Join(home, ".scanoss", "settings.json"))
	if _, ok := stored[KeyAPIKey]; ok {
		t.Error("api_key survived Unset")
	}
	if got := stored[KeyAPIURL]; got != "https://api.scanoss.com" {
		t.Errorf("api_url = %v, want it preserved", got)
	}
}

// Removing something that is not there already satisfies the request.
func TestUnsetAbsentKeySucceeds(t *testing.T) {
	withHome(t)
	if err := Unset(KeyAPIKey); err != nil {
		t.Errorf("Unset() on an absent key returned %v, want nil", err)
	}
}

func TestUnsetRejectsUnrecognizedKey(t *testing.T) {
	withHome(t)
	var unknown *UnknownKeyError
	if err := Unset("api_token"); !errors.As(err, &unknown) {
		t.Errorf("Unset() error = %v, want *UnknownKeyError", err)
	}
}

func TestSetOverwritesInPlace(t *testing.T) {
	home := withHome(t)
	if err := Set(KeyAPIKey, "SC_first"); err != nil {
		t.Fatal(err)
	}
	if err := Set(KeyAPIKey, "SC_second"); err != nil {
		t.Fatal(err)
	}

	stored := readFileMap(t, filepath.Join(home, ".scanoss", "settings.json"))
	if got := stored[KeyAPIKey]; got != "SC_second" {
		t.Errorf("stored api_key = %v, want %q", got, "SC_second")
	}
}
