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

	if err := runConfigSet(nil, []string{"api_key", "SC_abc123"}); err != nil {
		t.Fatalf("config set: %v", err)
	}
	if got := storedSettings(t, home)["api_key"]; got != "SC_abc123" {
		t.Errorf("stored api_key = %v, want %q", got, "SC_abc123")
	}
}

// A stored api_url must be normalized the way the SDK and the no-key guard expect,
// so a pasted URL with a trailing slash does not become a second distinct endpoint.
func TestConfigSetNormalizesURL(t *testing.T) {
	home := configHome(t)

	if err := runConfigSet(nil, []string{"api_url", "  https://scanoss.internal.example.com/  "}); err != nil {
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
	for _, key := range cliconfig.RecognizedKeys() {
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
		err := runConfigSet(nil, []string{"api_key", value})
		if err == nil {
			t.Fatalf("config set api_key %q succeeded, want an error", value)
		}
		if !strings.Contains(err.Error(), "config unset api_key") {
			t.Errorf("error %q does not point at `config unset api_key`", err)
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

	if err := runConfigSet(nil, []string{"api_key", "SC_abc123"}); err != nil {
		t.Fatal(err)
	}
	if err := runConfigSet(nil, []string{"api_url", "https://scanoss.internal.example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := runConfigUnset(nil, []string{"api_key"}); err != nil {
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
		"config set api_key",
		"config list",
		"config unset api_key",
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

	if err := runConfigSet(nil, []string{"api_key", "SC_stored"}); err != nil {
		t.Fatal(err)
	}
	if err := checkAuth(cmd); err != nil {
		t.Errorf("checkAuth failed with a stored key: %v", err)
	}
}
