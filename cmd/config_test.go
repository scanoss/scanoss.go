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
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/scanoss/scanoss.go/internal/cliconfig"
	"github.com/scanoss/scanoss.go/internal/config"
)

// configHome points HOME at a fresh temporary directory for one test. TestMain
// already redirects it for the package; this isolates each test from the others so
// one test's stored key cannot satisfy another's assertions.
func configHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

// storedSettings returns the settings file parsed as a map.
func storedSettings(t *testing.T, home string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".scanoss", "settings.json"))
	if err != nil {
		t.Fatalf("reading the settings file: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("parsing the settings file: %v", err)
	}
	return out
}

func TestConfigSet(t *testing.T) {
	home := configHome(t)

	if err := runConfigSet(nil, []string{"api-key", "SC_abc123"}); err != nil {
		t.Fatalf("config set: %v", err)
	}
	if got := storedSettings(t, home)["api_key"]; got != "SC_abc123" {
		t.Errorf("stored api_key = %v, want %q", got, "SC_abc123")
	}
}

// The command line has one spelling — the dashed one, matching every flag — and it is
// translated to the file's snake_case on the way in.
func TestConfigSetUsesCLISpelling(t *testing.T) {
	home := configHome(t)
	if err := runConfigSet(nil, []string{"api-key", "SC_abc123"}); err != nil {
		t.Fatalf("config set api-key: %v", err)
	}
	stored := storedSettings(t, home)
	if got := stored["api_key"]; got != "SC_abc123" {
		t.Errorf("stored under api_key = %v, want the value in snake_case", got)
	}
	if _, ok := stored["api-key"]; ok {
		t.Error("the dashed spelling reached the file")
	}
}

// The file's own spelling is not a second way to type a key: one way only, so there is
// one thing to document and one error to explain.
func TestConfigRejectsStoredSpelling(t *testing.T) {
	configHome(t)

	var unknown *cliconfig.UnknownKeyError
	if err := runConfigSet(nil, []string{"api_key", "SC_abc123"}); !errors.As(err, &unknown) {
		t.Errorf("config set api_key: error = %v, want *cliconfig.UnknownKeyError", err)
	}
	if err := runConfigUnset(nil, []string{"api_url"}); !errors.As(err, &unknown) {
		t.Errorf("config unset api_url: error = %v, want *cliconfig.UnknownKeyError", err)
	}
	if _, err := runConfigOut(t, runConfigGet, "api_key"); !errors.As(err, &unknown) {
		t.Errorf("config get api_key: error = %v, want *cliconfig.UnknownKeyError", err)
	}
	// A different case is a different spelling, and there is only one.
	if err := runConfigSet(nil, []string{"API-KEY", "SC_abc123"}); !errors.As(err, &unknown) {
		t.Errorf("config set API-KEY: error = %v, want *cliconfig.UnknownKeyError", err)
	}
}

// `list` prints the dashed spelling, so a line can be pasted straight back into a
// `config set`.
func TestConfigListUsesCLISpelling(t *testing.T) {
	configHome(t)

	out, err := runConfigOut(t, runConfigList)
	if err != nil {
		t.Fatalf("config list: %v", err)
	}
	for _, want := range []string{"api-key", "api-url"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output does not use the CLI spelling %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"api_key", "api_url"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("list output uses the stored spelling %q:\n%s", unwanted, out)
		}
	}
}

// A stored api_url must be normalized the way the SDK and the no-key guard expect,
// so a pasted URL with a trailing slash does not become a second distinct endpoint.
func TestConfigSetNormalizesURL(t *testing.T) {
	home := configHome(t)

	if err := runConfigSet(nil, []string{"api-url", "  https://scanoss.internal.example.com/  "}); err != nil {
		t.Fatalf("config set: %v", err)
	}
	if got := storedSettings(t, home)["api_url"]; got != "https://scanoss.internal.example.com" {
		t.Errorf("stored api_url = %v, want it trimmed with no trailing slash", got)
	}
}

func TestConfigSetRejectsUnrecognizedKey(t *testing.T) {
	home := configHome(t)

	err := runConfigSet(nil, []string{"api_token", "SC_abc123"})
	var unknown *cliconfig.UnknownKeyError
	if !errors.As(err, &unknown) {
		t.Fatalf("config set: error = %v, want *cliconfig.UnknownKeyError", err)
	}
	for _, key := range []string{"api-key", "api-url"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q does not list %q", err, key)
		}
	}
	if _, statErr := os.Stat(filepath.Join(home, ".scanoss")); !errors.Is(statErr, os.ErrNotExist) {
		t.Error("a rejected key created the settings directory")
	}
}

// Storing an empty value would read back as unset — a silent no-op. The error must
// point at the command that expresses the intent instead.
func TestConfigSetRejectsEmptyValue(t *testing.T) {
	configHome(t)

	for _, value := range []string{"", "   "} {
		err := runConfigSet(nil, []string{"api-key", value})
		if err == nil {
			t.Fatalf("config set api-key %q succeeded, want an error", value)
		}
		if !strings.Contains(err.Error(), "config unset api-key") {
			t.Errorf("error %q does not point at `config unset api-key`", err)
		}
	}
}

// An unrecognized key must be reported as such even when its value is empty: the
// typo is the useful thing to say.
func TestConfigSetUnknownKeyWinsOverEmptyValue(t *testing.T) {
	configHome(t)

	var unknown *cliconfig.UnknownKeyError
	if err := runConfigSet(nil, []string{"api_token", ""}); !errors.As(err, &unknown) {
		t.Errorf("error = %v, want *cliconfig.UnknownKeyError", err)
	}
}

func TestConfigUnset(t *testing.T) {
	home := configHome(t)

	if err := runConfigSet(nil, []string{"api-key", "SC_abc123"}); err != nil {
		t.Fatal(err)
	}
	if err := runConfigSet(nil, []string{"api-url", "https://scanoss.internal.example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := runConfigUnset(nil, []string{"api-key"}); err != nil {
		t.Fatalf("config unset: %v", err)
	}

	stored := storedSettings(t, home)
	if _, ok := stored["api_key"]; ok {
		t.Error("api_key survived config unset")
	}
	if got := stored["api_url"]; got != "https://scanoss.internal.example.com" {
		t.Errorf("api_url = %v, want it preserved", got)
	}
}

func TestConfigUnsetRejectsUnrecognizedKey(t *testing.T) {
	configHome(t)

	var unknown *cliconfig.UnknownKeyError
	if err := runConfigUnset(nil, []string{"api_token"}); !errors.As(err, &unknown) {
		t.Errorf("error = %v, want *cliconfig.UnknownKeyError", err)
	}
}

// runConfigOut invokes a config subcommand's RunE and returns what it wrote to
// stdout.
func runConfigOut(t *testing.T, run func(*cobra.Command, []string) error, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	err := run(cmd, args)
	return out.String(), err
}

func TestConfigGet(t *testing.T) {
	configHome(t)
	if err := runConfigSet(nil, []string{"api-url", "https://scanoss.internal.example.com"}); err != nil {
		t.Fatal(err)
	}

	out, err := runConfigOut(t, runConfigGet, "api-url")
	if err != nil {
		t.Fatalf("config get api_url: %v", err)
	}
	// Bare value, no label and no decoration: `get` has to compose in a shell.
	if out != "https://scanoss.internal.example.com\n" {
		t.Errorf("output = %q, want the bare value", out)
	}
}

// `get` reports the effective value, not the stored one, so it answers the question
// the user is actually asking: what will the next command use?
func TestConfigGetReportsEffectiveValue(t *testing.T) {
	configHome(t)
	if err := runConfigSet(nil, []string{"api-url", "https://file.example.com"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SCANOSS_API_URL", "https://env.example.com")

	out, err := runConfigOut(t, runConfigGet, "api-url")
	if err != nil {
		t.Fatalf("config get api_url: %v", err)
	}
	if strings.TrimSpace(out) != "https://env.example.com" {
		t.Errorf("output = %q, want the environment value", out)
	}
}

// An unset key exits non-zero rather than printing an empty line, so `if
// scanoss-cli config get api_key` is a usable check.
func TestConfigGetUnsetKeyFails(t *testing.T) {
	configHome(t)

	out, err := runConfigOut(t, runConfigGet, "api-key")
	if err == nil {
		t.Fatal("config get api-key succeeded with nothing stored, want an error")
	}
	if !strings.Contains(err.Error(), "api-key is not set") {
		t.Errorf("error = %q, want it to say the key is not set", err)
	}
	if out != "" {
		t.Errorf("output = %q, want nothing", out)
	}
}

func TestConfigGetRejectsUnrecognizedKey(t *testing.T) {
	configHome(t)

	var unknown *cliconfig.UnknownKeyError
	if _, err := runConfigOut(t, runConfigGet, "api_token"); !errors.As(err, &unknown) {
		t.Errorf("error = %v, want *cliconfig.UnknownKeyError", err)
	}
}

// The whole point of the masking rule: the key's value must not appear in any output
// path. This asserts the literal value is absent from the stream rather than merely
// that asterisks are present — a partial reveal would satisfy the weaker check.
func TestSecretNeverAppearsInOutput(t *testing.T) {
	configHome(t)
	const secret = "SC_this_must_never_be_printed"
	if err := runConfigSet(nil, []string{"api-key", secret}); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		run  func(*cobra.Command, []string) error
		args []string
	}{
		{"get", runConfigGet, []string{"api-key"}},
		{"list", runConfigList, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runConfigOut(t, tc.run, tc.args...)
			if err != nil {
				t.Fatalf("config %s: %v", tc.name, err)
			}
			if strings.Contains(out, secret) {
				t.Errorf("config %s printed the api_key value: %q", tc.name, out)
			}
			// A prefix long enough to be useful to an attacker must not leak either.
			if strings.Contains(out, secret[:8]) {
				t.Errorf("config %s leaked a prefix of the api_key: %q", tc.name, out)
			}
			if !strings.Contains(out, maskedValue) {
				t.Errorf("config %s did not mask the api_key: %q", tc.name, out)
			}
		})
	}
}

// The mask must not reveal the length either, so two keys of very different sizes
// have to render identically.
func TestMaskIsConstantWidth(t *testing.T) {
	configHome(t)

	var renders []string
	for _, secret := range []string{"a", strings.Repeat("x", 128)} {
		if err := runConfigSet(nil, []string{"api-key", secret}); err != nil {
			t.Fatal(err)
		}
		out, err := runConfigOut(t, runConfigGet, "api-key")
		if err != nil {
			t.Fatal(err)
		}
		renders = append(renders, out)
	}
	if renders[0] != renders[1] {
		t.Errorf("mask varies with the value: %q vs %q", renders[0], renders[1])
	}
}

// On a fresh machine `list` must still explain itself: an empty report would read as
// a broken command, and the recognized keys double as a reference.
func TestConfigListOnFreshMachine(t *testing.T) {
	home := configHome(t)

	out, err := runConfigOut(t, runConfigList)
	if err != nil {
		t.Fatalf("config list: %v", err)
	}
	for _, want := range []string{
		"api-key", "(unset)",
		"api-url", config.DefaultAPIURL, "(default)",
		"Config file: " + filepath.Join(home, ".scanoss", "settings.json"),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("list output does not contain %q:\n%s", want, out)
		}
	}
	// No trailing whitespace: the report is something people diff and paste.
	for _, line := range strings.Split(out, "\n") {
		if line != strings.TrimRight(line, " \t") {
			t.Errorf("line has trailing whitespace: %q", line)
		}
	}
}

// The source is load-bearing, not decoration: with the value masked, it is the only
// way to tell a stored key from one the environment supplied.
func TestConfigListReportsSources(t *testing.T) {
	configHome(t)
	if err := runConfigSet(nil, []string{"api-key", "SC_stored"}); err != nil {
		t.Fatal(err)
	}
	if err := runConfigSet(nil, []string{"api-url", "https://file.example.com"}); err != nil {
		t.Fatal(err)
	}

	out, err := runConfigOut(t, runConfigList)
	if err != nil {
		t.Fatalf("config list: %v", err)
	}
	if !strings.Contains(out, "(config file)") {
		t.Errorf("list does not report the config-file source:\n%s", out)
	}

	t.Setenv("SCANOSS_API_KEY", "from_env")
	out, err = runConfigOut(t, runConfigList)
	if err != nil {
		t.Fatalf("config list: %v", err)
	}
	// Naming the variable answers the follow-up question: which one do I unset?
	if !strings.Contains(out, "(env: SCANOSS_API_KEY)") {
		t.Errorf("list does not name the environment variable that won:\n%s", out)
	}
}

// A hand-edited key this version does not recognize must be visible, not silently
// ignored — otherwise `list` implies the file holds less than it does.
func TestConfigListShowsUnrecognizedKeys(t *testing.T) {
	home := configHome(t)
	dir := filepath.Join(home, ".scanoss")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := `{"api_key": "SC_stored", "future_setting": "kept"}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := runConfigOut(t, runConfigList)
	if err != nil {
		t.Fatalf("config list: %v", err)
	}
	for _, want := range []string{"future_setting", "kept", "unrecognized"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output does not contain %q:\n%s", want, out)
		}
	}
}

func TestConfigPath(t *testing.T) {
	home := configHome(t)

	out, err := runConfigOut(t, runConfigPath)
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	want := filepath.Join(home, ".scanoss", "settings.json")
	if strings.TrimSpace(out) != want {
		t.Errorf("output = %q, want %q", strings.TrimSpace(out), want)
	}
	// The path must be printed whether or not the file exists, and printing it must
	// not create anything.
	if _, statErr := os.Stat(filepath.Join(home, ".scanoss")); !errors.Is(statErr, os.ErrNotExist) {
		t.Error("config path created the settings directory")
	}
}

// The command must be reachable and self-documenting: `config` with no subcommand
// shows help, and the help text names the precedence chain and both env vars.
func TestConfigCommandHelp(t *testing.T) {
	if configCmd.Long == "" {
		t.Fatal("configCmd has no Long help text")
	}
	for _, want := range []string{
		"api_url", "api_key",
		"SCANOSS_API_URL", "SCANOSS_API_KEY",
		"flag > environment variable > config file",
		"********",
		"scanoss.json",
		"Examples:",
		"config set api-key",
		"config list",
		"config unset api-key",
		"config path",
	} {
		if !strings.Contains(configCmd.Long, want) {
			t.Errorf("config help does not mention %q", want)
		}
	}

	if _, _, err := rootCmd.Find([]string{"config", "set"}); err != nil {
		t.Errorf("`config set` is not registered: %v", err)
	}
	if _, _, err := rootCmd.Find([]string{"config", "unset"}); err != nil {
		t.Errorf("`config unset` is not registered: %v", err)
	}
}

// A stored key must satisfy the no-key guard: the banner exists to tell a user they
// have no credentials, and a user with credentials on disk is not that user.
func TestStoredKeySatisfiesAuthGuard(t *testing.T) {
	configHome(t)

	cmd := &cobra.Command{}
	cmd.Flags().String("api-url", config.DefaultAPIURL, "")
	cmd.Flags().String("api-key", "", "")

	if err := checkAuth(cmd); err == nil {
		t.Fatal("checkAuth passed with no key on the default endpoint")
	}

	if err := runConfigSet(nil, []string{"api-key", "SC_stored"}); err != nil {
		t.Fatal(err)
	}
	if err := checkAuth(cmd); err != nil {
		t.Errorf("checkAuth failed with a stored key: %v", err)
	}
}
