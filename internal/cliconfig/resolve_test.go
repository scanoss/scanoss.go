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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

const defaultAPIURL = "https://api.scanoss.com"

// apiFlags builds the flag set the real commands declare: api-url carries a
// non-empty default, api-key does not.
func apiFlags(t *testing.T) *pflag.FlagSet {
	t.Helper()
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("api-url", defaultAPIURL, "")
	flags.String("api-key", "", "")
	return flags
}

// setFlag marks a flag as explicitly typed by the user, which is what earns it the
// top rung.
func setFlag(t *testing.T, flags *pflag.FlagSet, name, value string) {
	t.Helper()
	if err := flags.Set(name, value); err != nil {
		t.Fatalf("setting --%s: %v", name, err)
	}
}

// writeSettings puts a settings file in the test's temporary home.
func writeSettings(t *testing.T, home, body string) string {
	t.Helper()
	dir := filepath.Join(home, ".scanoss")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestResolverValueAndSourceAgree walks every rung of the ladder. viper produces
// the value and walk names the source; this is the test that keeps the two
// encodings of the precedence chain from drifting apart.
func TestResolverValueAndSourceAgree(t *testing.T) {
	cases := []struct {
		name       string
		file       string
		env        map[string]string
		flag       map[string]string
		wantURL    Setting
		wantAPIKey Setting
	}{
		{
			name:       "nothing configured",
			wantURL:    Setting{Value: defaultAPIURL, Source: SourceDefault},
			wantAPIKey: Setting{Source: SourceUnset},
		},
		{
			name:       "config file only",
			file:       `{"api_url": "https://file.example.com", "api_key": "from_file"}`,
			wantURL:    Setting{Value: "https://file.example.com", Source: SourceFile},
			wantAPIKey: Setting{Value: "from_file", Source: SourceFile},
		},
		{
			name:       "environment overrides the file",
			file:       `{"api_url": "https://file.example.com", "api_key": "from_file"}`,
			env:        map[string]string{"SCANOSS_API_URL": "https://env.example.com", "SCANOSS_API_KEY": "from_env"},
			wantURL:    Setting{Value: "https://env.example.com", Source: SourceEnv},
			wantAPIKey: Setting{Value: "from_env", Source: SourceEnv},
		},
		{
			name:       "flag overrides the environment and the file",
			file:       `{"api_url": "https://file.example.com", "api_key": "from_file"}`,
			env:        map[string]string{"SCANOSS_API_URL": "https://env.example.com", "SCANOSS_API_KEY": "from_env"},
			flag:       map[string]string{"api-url": "https://flag.example.com", "api-key": "from_flag"},
			wantURL:    Setting{Value: "https://flag.example.com", Source: SourceFlag},
			wantAPIKey: Setting{Value: "from_flag", Source: SourceFlag},
		},
		{
			name:       "an empty environment variable falls through to the file",
			file:       `{"api_url": "https://file.example.com", "api_key": "from_file"}`,
			env:        map[string]string{"SCANOSS_API_URL": "", "SCANOSS_API_KEY": ""},
			wantURL:    Setting{Value: "https://file.example.com", Source: SourceFile},
			wantAPIKey: Setting{Value: "from_file", Source: SourceFile},
		},
		{
			name:       "an empty file value falls through to the flag default",
			file:       `{"api_url": "", "api_key": ""}`,
			wantURL:    Setting{Value: defaultAPIURL, Source: SourceDefault},
			wantAPIKey: Setting{Source: SourceUnset},
		},
		{
			name:       "a file value for one key does not affect the other",
			file:       `{"api_key": "from_file"}`,
			wantURL:    Setting{Value: defaultAPIURL, Source: SourceDefault},
			wantAPIKey: Setting{Value: "from_file", Source: SourceFile},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := withHome(t)
			if tc.file != "" {
				writeSettings(t, home, tc.file)
			}
			for name, value := range tc.env {
				t.Setenv(name, value)
			}
			flags := apiFlags(t)
			for name, value := range tc.flag {
				setFlag(t, flags, name, value)
			}

			r, err := newResolver(flags)
			if err != nil {
				t.Fatalf("newResolver() error: %v", err)
			}

			// The table states the value and the source; Key is always the key asked
			// for, asserted once below rather than repeated in every case.
			for _, tc := range []struct {
				key  string
				want Setting
			}{{KeyAPIURL, tc.wantURL}, {KeyAPIKey, tc.wantAPIKey}} {
				got := r.Key(tc.key)
				if got.Value != tc.want.Value || got.Source != tc.want.Source {
					t.Errorf("Key(%s) = {%q, %s}, want {%q, %s}",
						tc.key, got.Value, got.Source, tc.want.Value, tc.want.Source)
				}
				if got.Key != tc.key {
					t.Errorf("Key(%s) reported key %q", tc.key, got.Key)
				}
			}

			// The value viper resolves must match the value reported alongside the
			// source. If the two ladders ever disagree, this is what fails.
			if want := r.viper.GetString(KeyAPIURL); tc.wantURL.Source != SourceDefault && want != tc.wantURL.Value {
				t.Errorf("viper resolved api_url as %q but the reported value is %q", want, tc.wantURL.Value)
			}
			if want := r.viper.GetString(KeyAPIKey); tc.wantAPIKey.Source != SourceUnset && want != tc.wantAPIKey.Value {
				t.Errorf("viper resolved api_key as %q but the reported value is %q", want, tc.wantAPIKey.Value)
			}
		})
	}
}

func TestResolveAPI(t *testing.T) {
	home := withHome(t)
	writeSettings(t, home, `{"api_key": "from_file"}`)
	t.Setenv("SCANOSS_API_URL", "https://env.example.com")

	api, err := ResolveAPI(apiFlags(t))
	if err != nil {
		t.Fatalf("ResolveAPI() error: %v", err)
	}
	if api.URL != "https://env.example.com" {
		t.Errorf("URL = %q, want the environment value", api.URL)
	}
	if api.Key != "from_file" {
		t.Errorf("Key = %q, want the stored value", api.Key)
	}
}

// A command that declares neither flag must still resolve from the environment and
// the file, and must not error.
func TestResolveAPIWithoutFlags(t *testing.T) {
	home := withHome(t)
	writeSettings(t, home, `{"api_url": "https://file.example.com"}`)

	api, err := ResolveAPI(pflag.NewFlagSet("empty", pflag.ContinueOnError))
	if err != nil {
		t.Fatalf("ResolveAPI() error: %v", err)
	}
	if api.URL != "https://file.example.com" {
		t.Errorf("URL = %q, want the stored value", api.URL)
	}
	if api.Key != "" {
		t.Errorf("Key = %q, want empty", api.Key)
	}
}

// A nil flag set is what a command with no flags at all effectively passes; it must
// not panic.
func TestResolveAPINilFlags(t *testing.T) {
	home := withHome(t)
	writeSettings(t, home, `{"api_key": "from_file"}`)

	api, err := ResolveAPI(nil)
	if err != nil {
		t.Fatalf("ResolveAPI(nil) error: %v", err)
	}
	if api.Key != "from_file" {
		t.Errorf("Key = %q, want the stored value", api.Key)
	}
}

// A malformed file must fail the command rather than resolve to defaults, so a
// broken config cannot silently retarget a scan.
func TestResolveAPIMalformedFile(t *testing.T) {
	home := withHome(t)
	path := writeSettings(t, home, `{"api_key": }`)

	_, err := ResolveAPI(apiFlags(t))
	if err == nil {
		t.Fatal("ResolveAPI() succeeded on a malformed file, want an error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not name %q", err, path)
	}
}

// An explicit --api-key "" must win over a stored key: passing an empty flag is how
// a user opts out of the stored credential for one run.
func TestExplicitEmptyFlagOverridesFile(t *testing.T) {
	home := withHome(t)
	writeSettings(t, home, `{"api_key": "from_file"}`)

	flags := apiFlags(t)
	setFlag(t, flags, "api-key", "")

	r, err := newResolver(flags)
	if err != nil {
		t.Fatalf("newResolver() error: %v", err)
	}
	got := r.Key(KeyAPIKey)
	if got.Source != SourceFlag || got.Value != "" {
		t.Errorf("Key(api_key) = %+v, want an empty value from the flag", got)
	}
}

func TestCLIKey(t *testing.T) {
	if got, want := CLIKey(KeyAPIURL), "api-url"; got != want {
		t.Errorf("CLIKey(%q) = %q, want %q", KeyAPIURL, got, want)
	}
	if got, want := CLIKey(KeyAPIKey), "api-key"; got != want {
		t.Errorf("CLIKey(%q) = %q, want %q", KeyAPIKey, got, want)
	}
	if got := CLIKey("api_token"); got != "" {
		t.Errorf("CLIKey(unrecognized) = %q, want empty", got)
	}
}

// transportFlags builds the flag set the real commands declare for the transport
// pair: both default to empty.
func transportFlags(t *testing.T) *pflag.FlagSet {
	t.Helper()
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("proxy", "", "")
	flags.String("ca-cert", "", "")
	return flags
}

// The transport pair walks the same ladder as the API pair, so this mirrors
// TestResolverValueAndSourceAgree for the two new keys.
func TestResolveTransport(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		env     map[string]string
		flag    map[string]string
		wantVia string
		wantCA  string
	}{
		{
			name: "nothing configured",
		},
		{
			name:    "config file",
			file:    `{"proxy": "http://file.example.com:8080", "ca_cert": "/file/ca.pem"}`,
			wantVia: "http://file.example.com:8080",
			wantCA:  "/file/ca.pem",
		},
		{
			name:    "environment overrides the file",
			file:    `{"proxy": "http://file.example.com:8080", "ca_cert": "/file/ca.pem"}`,
			env:     map[string]string{"SCANOSS_PROXY": "http://env.example.com:8080", "SCANOSS_CA_CERT": "/env/ca.pem"},
			wantVia: "http://env.example.com:8080",
			wantCA:  "/env/ca.pem",
		},
		{
			name:    "flag overrides both",
			file:    `{"proxy": "http://file.example.com:8080", "ca_cert": "/file/ca.pem"}`,
			env:     map[string]string{"SCANOSS_PROXY": "http://env.example.com:8080", "SCANOSS_CA_CERT": "/env/ca.pem"},
			flag:    map[string]string{"proxy": "http://flag.example.com:8080", "ca-cert": "/flag/ca.pem"},
			wantVia: "http://flag.example.com:8080",
			wantCA:  "/flag/ca.pem",
		},
		{
			name:   "one key stored does not affect the other",
			file:   `{"ca_cert": "/file/ca.pem"}`,
			wantCA: "/file/ca.pem",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := withHome(t)
			if tc.file != "" {
				writeSettings(t, home, tc.file)
			}
			for name, value := range tc.env {
				t.Setenv(name, value)
			}
			flags := transportFlags(t)
			for name, value := range tc.flag {
				setFlag(t, flags, name, value)
			}

			got, err := ResolveTransport(flags)
			if err != nil {
				t.Fatalf("ResolveTransport() error: %v", err)
			}
			if got.Proxy != tc.wantVia {
				t.Errorf("Proxy = %q, want %q", got.Proxy, tc.wantVia)
			}
			if got.CACertFile != tc.wantCA {
				t.Errorf("CACertFile = %q, want %q", got.CACertFile, tc.wantCA)
			}
		})
	}
}

// A command that declares neither flag — `config list` is one — must still resolve
// from the environment and the file.
func TestResolveTransportWithoutFlags(t *testing.T) {
	home := withHome(t)
	writeSettings(t, home, `{"ca_cert": "/file/ca.pem"}`)

	got, err := ResolveTransport(pflag.NewFlagSet("empty", pflag.ContinueOnError))
	if err != nil {
		t.Fatalf("ResolveTransport() error: %v", err)
	}
	if got.CACertFile != "/file/ca.pem" {
		t.Errorf("CACertFile = %q, want the stored value", got.CACertFile)
	}
}
