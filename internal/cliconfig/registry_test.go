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
	"strings"
	"testing"
)

func TestRegistry(t *testing.T) {
	if got, want := strings.Join(recognizedKeys(), ","), "api_key,api_url,ca_cert,proxy"; got != want {
		t.Errorf("recognizedKeys() = %q, want %q (sorted)", got, want)
	}
	for _, key := range []string{KeyAPIURL, KeyAPIKey, KeyProxy, KeyCACert} {
		if !IsRecognized(key) {
			t.Errorf("IsRecognized(%q) = false, want true", key)
		}
	}
	if IsRecognized("api_token") {
		t.Error(`IsRecognized("api_token") = true, want false`)
	}
	if !IsSecret(KeyAPIKey) {
		t.Error("api_key must be marked secret")
	}
	if IsSecret(KeyAPIURL) {
		t.Error("api_url must not be marked secret")
	}
	if IsSecret("api_token") {
		t.Error("an unrecognized key must not report as secret")
	}
	if got, want := EnvName(KeyAPIKey), "SCANOSS_API_KEY"; got != want {
		t.Errorf("EnvName(%q) = %q, want %q", KeyAPIKey, got, want)
	}
	if got, want := EnvName(KeyAPIURL), "SCANOSS_API_URL"; got != want {
		t.Errorf("EnvName(%q) = %q, want %q", KeyAPIURL, got, want)
	}

	// A proxy URL and a file path are not credentials, so they are shown in full.
	for _, key := range []string{KeyProxy, KeyCACert} {
		if IsSecret(key) {
			t.Errorf("%s must not be marked secret", key)
		}
		// No built-in default: absent means "use the environment, or the system pool".
		if def := defaultOf(key); def != "" {
			t.Errorf("Default(%q) = %q, want empty", key, def)
		}
	}
	if got, want := EnvName(KeyProxy), "SCANOSS_PROXY"; got != want {
		t.Errorf("EnvName(%q) = %q, want %q", KeyProxy, got, want)
	}
	if got, want := EnvName(KeyCACert), "SCANOSS_CA_CERT"; got != want {
		t.Errorf("EnvName(%q) = %q, want %q", KeyCACert, got, want)
	}
}

// The CLI has exactly one spelling per setting: the dashed one. The stored
// snake_case form is the file format, not a second way to type a key.
func TestStoredKey(t *testing.T) {
	for _, input := range []string{"api-key", "  api-key  "} {
		stored, ok := StoredKey(input)
		if !ok {
			t.Errorf("StoredKey(%q) reported unrecognized", input)
			continue
		}
		if stored != KeyAPIKey {
			t.Errorf("StoredKey(%q) = %q, want %q", input, stored, KeyAPIKey)
		}
	}
	// One way only: the file spelling, a different case, and a typo are all rejected,
	// so there is one thing to document and one error to explain.
	// ca-cert is the one key whose stored and command-line spellings differ by more
	// than a dash, so it is worth asserting on its own.
	if stored, ok := StoredKey("ca-cert"); !ok || stored != KeyCACert {
		t.Errorf(`StoredKey("ca-cert") = (%q, %t), want (%q, true)`, stored, ok, KeyCACert)
	}
	if stored, ok := StoredKey("proxy"); !ok || stored != KeyProxy {
		t.Errorf(`StoredKey("proxy") = (%q, %t), want (%q, true)`, stored, ok, KeyProxy)
	}
	// One way only: the file spelling, a different case, and a typo are all rejected,
	// so there is one thing to document and one error to explain.
	for _, input := range []string{"api_key", "API-KEY", "Api_Key", "api-token", "apikey", "ca_cert", "cacert"} {
		if stored, ok := StoredKey(input); ok {
			t.Errorf("StoredKey(%q) = (%q, true), want unrecognized", input, stored)
		}
	}
}

func TestCLIKeyAndCLIKeys(t *testing.T) {
	if got, want := CLIKey(KeyAPIKey), "api-key"; got != want {
		t.Errorf("CLIKey(%q) = %q, want %q", KeyAPIKey, got, want)
	}
	if got, want := CLIKey(KeyAPIURL), "api-url"; got != want {
		t.Errorf("CLIKey(%q) = %q, want %q", KeyAPIURL, got, want)
	}
	if got := CLIKey("api_token"); got != "" {
		t.Errorf("CLIKey(unrecognized) = %q, want empty", got)
	}
	if got, want := CLIKey(KeyCACert), "ca-cert"; got != want {
		t.Errorf("CLIKey(%q) = %q, want %q", KeyCACert, got, want)
	}
	if got, want := CLIKey(KeyProxy), "proxy"; got != want {
		t.Errorf("CLIKey(%q) = %q, want %q", KeyProxy, got, want)
	}
	if got, want := strings.Join(cliKeys(), ","), "api-key,api-url,ca-cert,proxy"; got != want {
		t.Errorf("cliKeys() = %q, want %q (sorted)", got, want)
	}
}
