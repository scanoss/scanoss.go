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

package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/scanoss/scanoss.go/pkg/settings"
)

// The legacy SCANOSS component list is what scanoss.py's --identify takes, and the shape is
// detected from content rather than the file name — so the same file works under any extension.
func TestParseComponentListFormats(t *testing.T) {
	tests := []struct {
		name string
		data string
		want []string
	}{
		{
			"legacy scanoss list",
			`{"components":[{"purl":"pkg:npm/vue@2.6.14"},{"purl":"pkg:npm/lodash"}]}`,
			[]string{"pkg:npm/vue@2.6.14", "pkg:npm/lodash"},
		},
		{
			"cyclonedx",
			`{"bomFormat":"CycloneDX","specVersion":"1.6","version":1,"components":[
				{"type":"library","name":"vue","version":"2.6.14","purl":"pkg:npm/vue@2.6.14"}]}`,
			[]string{"pkg:npm/vue@2.6.14"},
		},
		{
			"duplicates collapse, blanks drop",
			`{"components":[{"purl":"pkg:npm/vue"},{"purl":"pkg:npm/vue"},{"purl":""},{"purl":"  "}]}`,
			[]string{"pkg:npm/vue"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseComponentList([]byte(tt.data))
			if err != nil {
				t.Fatalf("parseComponentList: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("purl %d = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseComponentListRejectsUnknownShapes(t *testing.T) {
	for _, data := range []string{`{"nothing":"useful"}`, `[1,2,3]`, `not json at all`} {
		if _, err := parseComponentList([]byte(data)); err == nil {
			t.Errorf("parseComponentList(%q) should have failed", data)
		}
	}
}

// A file that exists but names no component is an error, not an empty rule set: silently
// scanning as if the flag were absent would hide a typo in the user's SBOM.
func TestReadComponentListRejectsEmptyInput(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		"empty file":  "",
		"no purls":    `{"components":[]}`,
		"blank purls": `{"components":[{"purl":""}]}`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, "list.json")
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := readComponentList(path, "--identify"); err == nil {
				t.Error("expected an error")
			}
		})
	}

	if _, err := readComponentList(filepath.Join(dir, "missing.json"), "--identify"); err == nil {
		t.Error("a missing file should be an error")
	}
}

// bomRuleCmd is a command carrying just the flags under test.
func bomRuleCmd(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error { return nil }}
	addBOMRuleFlags(cmd)
	addSkipHeaderFlags(cmd)
	addRankingFlag(cmd)
	cmd.SetArgs(args)
	cmd.SetOut(os.NewFile(0, os.DevNull))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("parsing %v: %v", args, err)
	}
	return cmd
}

// The flags add to whatever the settings file already said, rather than replacing it. Unlike
// scanoss.py, which refuses --identify alongside --settings, the two are merged.
func TestMergeBOMRuleFlagsAddsToTheSettings(t *testing.T) {
	dir := t.TempDir()
	listPath := filepath.Join(dir, "sbom.json")
	if err := os.WriteFile(listPath, []byte(`{"components":[{"purl":"pkg:npm/flagged"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	existing := &settings.Settings{BOM: settings.BOM{
		Identify: []settings.BOMEntry{{Purl: "pkg:npm/from-settings", Path: "src/"}},
	}}
	cmd := bomRuleCmd(t, "--identify", listPath)

	merged, err := mergeBOMRuleFlags(cmd, existing)
	if err != nil {
		t.Fatalf("mergeBOMRuleFlags: %v", err)
	}

	rules := merged.BOM.IdentifyRules()
	if len(rules) != 2 {
		t.Fatalf("got %d identify rules, want 2 (settings + flag): %+v", len(rules), rules)
	}
	// The flag contributes an unscoped rule, so it applies everywhere.
	for _, r := range rules {
		if r.Purl == "pkg:npm/flagged" && r.Path != "" {
			t.Errorf("the flag's rule should carry no path, got %q", r.Path)
		}
	}
}

// A project with no settings file still gets the flags honored.
func TestMergeBOMRuleFlagsWithoutSettings(t *testing.T) {
	dir := t.TempDir()
	listPath := filepath.Join(dir, "sbom.json")
	if err := os.WriteFile(listPath, []byte(`{"components":[{"purl":"pkg:npm/x"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	merged, err := mergeBOMRuleFlags(bomRuleCmd(t, "--ignore", listPath), nil)
	if err != nil {
		t.Fatalf("mergeBOMRuleFlags: %v", err)
	}
	if len(merged.BOM.IgnoreRules()) != 1 {
		t.Errorf("ignore rules = %+v, want one", merged.BOM.IgnoreRules())
	}
}

// Neither flag given leaves the settings exactly as they were, nil included.
func TestMergeBOMRuleFlagsNoOp(t *testing.T) {
	if got, err := mergeBOMRuleFlags(bomRuleCmd(t), nil); err != nil || got != nil {
		t.Errorf("got (%v, %v), want (nil, nil)", got, err)
	}
}

// scanoss.json wins over the command line here, matching scanoss.py. It is the reverse of the
// collection flags, and the reason this is pinned by a test.
func TestResolveSkipHeadersSettingsWinOverFlags(t *testing.T) {
	truth, limit := true, 25
	settingsSaysOn := &settings.Settings{Settings: settings.Tuning{
		FileSnippet: settings.FileSnippet{SkipHeaders: &truth, SkipHeadersLimit: &limit},
	}}
	falsity, zero := false, 0
	settingsSaysOff := &settings.Settings{Settings: settings.Tuning{
		FileSnippet: settings.FileSnippet{SkipHeaders: &falsity, SkipHeadersLimit: &zero},
	}}

	tests := []struct {
		name      string
		args      []string
		settings  *settings.Settings
		wantOn    bool
		wantLimit int
	}{
		// The filter is on by default: a licence header is boilerplate shared by every file
		// carrying it, so fingerprinting it makes unrelated files look alike.
		{"no flag, no settings", nil, nil, true, 0},
		{"flag alone", []string{"--skip-headers", "--skip-headers-limit", "10"}, nil, true, 10},
		{"the flag can turn it off", []string{"--skip-headers=false"}, nil, false, 0},
		{"settings agree with the default", nil, settingsSaysOn, true, 25},
		{"settings override the flag", []string{"--skip-headers", "--skip-headers-limit", "10"}, settingsSaysOn, true, 25},
		{"settings disable what the flag asked for", []string{"--skip-headers"}, settingsSaysOff, false, 0},
		// The settings file wins in both directions, including over the default.
		{"settings disable the default", nil, settingsSaysOff, false, 0},
		{"settings re-enable what the flag turned off", []string{"--skip-headers=false"}, settingsSaysOn, true, 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			on, limit := resolveSkipHeaders(bomRuleCmd(t, tt.args...), tt.settings)
			if on != tt.wantOn || limit != tt.wantLimit {
				t.Errorf("resolveSkipHeaders = (%v, %d), want (%v, %d)", on, limit, tt.wantOn, tt.wantLimit)
			}
		})
	}
}

func TestFingerprintOptionsOnlyWhenEnabled(t *testing.T) {
	if got := fingerprintOptions(false, 10); got != nil {
		t.Errorf("options with the filter off = %v, want nil", got)
	}
	if got := fingerprintOptions(true, 10); len(got) != 1 {
		t.Errorf("options with the filter on = %v, want one", got)
	}
}

func rankingSettings(threshold int) *settings.Settings {
	return &settings.Settings{Settings: settings.Tuning{
		FileSnippet: settings.FileSnippet{RankingThreshold: &threshold},
	}}
}

// scanoss.json wins over the command line here too, and out-of-range values are clamped rather
// than rejected.
func TestResolveRankingThreshold(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		settings *settings.Settings
		want     int
	}{
		{"default is off", nil, nil, -1},
		{"flag alone", []string{"--ranking-threshold", "5"}, nil, 5},
		{"zero is off", []string{"--ranking-threshold", "0"}, nil, 0},
		{"settings override the flag", []string{"--ranking-threshold", "5"}, rankingSettings(2), 2},
		{"settings enable what the flag did not", nil, rankingSettings(3), 3},
		// An explicit 0 or -1 in the settings file turns the filter off even though the flag
		// asked for it: the file wins either way.
		{"settings disable what the flag asked for", []string{"--ranking-threshold", "5"}, rankingSettings(0), 0},
		{"above the maximum clamps down", []string{"--ranking-threshold", "99"}, nil, settings.MaxRankingThreshold},
		{"below the minimum clamps up", []string{"--ranking-threshold", "-7"}, nil, settings.MinRankingThreshold},
		{"an out-of-range settings value clamps too", nil, rankingSettings(50), settings.MaxRankingThreshold},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveRankingThreshold(bomRuleCmd(t, tt.args...), tt.settings); got != tt.want {
				t.Errorf("resolveRankingThreshold = %d, want %d", got, tt.want)
			}
		})
	}
}
