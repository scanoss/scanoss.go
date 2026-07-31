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
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/scanoss/scanoss.go/pkg/scanoss"
)

// apiCommand builds a command carrying the flags addAPIFlags registers and parses
// the given ones into it.
//
// It goes through ParseFlags rather than Flags().Set because addAPIFlags registers
// persistent flags, and cobra only merges those into Flags() when it parses — the
// same path a real invocation takes.
func apiCommand(t *testing.T, flags map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	addAPIFlags(cmd)

	args := make([]string, 0, len(flags)*2)
	for name, value := range flags {
		args = append(args, "--"+name, value)
	}
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("parsing %v: %v", args, err)
	}
	return cmd
}

// captureWarnings collects slog output for the duration of a test.
func captureWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(original) })
	return &buf
}

// With no flags every transport field stays unset, which is what leaves the SDK on Go's
// defaults — and with them HTTP_PROXY, HTTPS_PROXY and NO_PROXY.
func TestAPIConfigDefaults(t *testing.T) {
	cfg, err := apiConfig(apiCommand(t, nil))
	if err != nil {
		t.Fatalf("apiConfig() error: %v", err)
	}
	if cfg.Proxy != "" || cfg.CACertFile != "" || cfg.InsecureTLS {
		t.Errorf("transport fields = %q/%q/%v, want all unset", cfg.Proxy, cfg.CACertFile, cfg.InsecureTLS)
	}
}

// Each transport flag reaches the Config field that carries it. What the SDK then builds
// from those fields — the proxy resolver, the certificate pool — is pkg/scanoss's own
// test; this only pins the handover.
func TestAPIConfigCarriesTransportFlags(t *testing.T) {
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caPath, []byte("unused"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := apiConfig(apiCommand(t, map[string]string{
		"proxy":   "http://proxy.example.com:8080",
		"ca-cert": caPath,
	}))
	if err != nil {
		t.Fatalf("apiConfig() error: %v", err)
	}
	if cfg.Proxy != "http://proxy.example.com:8080" {
		t.Errorf("Proxy = %q, want the flag value", cfg.Proxy)
	}
	if cfg.CACertFile != caPath {
		t.Errorf("CACertFile = %q, want %q", cfg.CACertFile, caPath)
	}
}

// A bad flag value must stop the command rather than reach the network. The flags are
// resolved here and validated when the client is built, so the guarantee spans both.
func TestAPIConfigRejectsBadFlags(t *testing.T) {
	for name, flags := range map[string]map[string]string{
		"proxy without a scheme": {"proxy": "proxy.example.com:8080"},
		"missing CA file":        {"ca-cert": filepath.Join(t.TempDir(), "nope.pem")},
	} {
		t.Run(name, func(t *testing.T) {
			cfg, err := apiConfig(apiCommand(t, flags))
			if err == nil {
				_, err = scanoss.New(cfg)
			}
			if err == nil {
				t.Errorf("%v was accepted, want an error", flags)
			}
		})
	}
}

// Passing both is pointless rather than wrong, so it warns instead of failing —
// but it has to say so, or the CA flag looks effective when nothing reads it.
func TestAPIHTTPClientWarnsWhenCACertIsIgnored(t *testing.T) {
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caPath, []byte("unused"), 0o600); err != nil {
		t.Fatal(err)
	}

	// The file deliberately holds no certificate: on its own that is an error, so a
	// run that succeeds here proves the CA was genuinely dropped rather than merely
	// unused.
	t.Run("both flags warn, and the CA file is not read", func(t *testing.T) {
		logged := captureWarnings(t)
		cfg, err := apiConfig(apiCommand(t, map[string]string{
			"ca-cert":            caPath,
			"ignore-cert-errors": "true",
		}))
		if err != nil {
			t.Fatalf("apiConfig() error: %v", err)
		}
		// Dropped, not merely unused: the file holds no certificate, so a CACertFile
		// that survived here would fail when the client was built.
		if cfg.CACertFile != "" {
			t.Errorf("CACertFile = %q, want it dropped", cfg.CACertFile)
		}
		if _, err := scanoss.New(cfg); err != nil {
			t.Fatalf("New() with the CA dropped: %v", err)
		}
		out := logged.String()
		if !strings.Contains(out, "ignoring TLS certificate errors") {
			t.Errorf("the insecure warning is missing:\n%s", out)
		}
		// The setting is named without a leading dash: it can come from the config
		// file, so calling it "--ca-cert" would name a flag the user never passed.
		if !strings.Contains(out, "ca-cert has no effect") {
			t.Errorf("the ca-cert warning is missing:\n%s", out)
		}
	})

	// A file that holds no certificate is normally an error; with verification off
	// it is never read, so the command must still run.
	t.Run("insecure alone does not warn about the CA", func(t *testing.T) {
		logged := captureWarnings(t)
		if _, err := apiConfig(apiCommand(t, map[string]string{"ignore-cert-errors": "true"})); err != nil {
			t.Fatalf("apiConfig() error: %v", err)
		}
		if strings.Contains(logged.String(), "ca-cert") {
			t.Errorf("warned about ca-cert when it was not set:\n%s", logged.String())
		}
	})
}

// A stored setting must reach the Config with no flag passed — the whole point of making
// proxy and ca-cert config keys.
func TestAPIConfigUsesStoredSettings(t *testing.T) {
	configHome(t)
	if err := runConfigSet(nil, []string{"proxy", "http://stored.example.com:8080"}); err != nil {
		t.Fatal(err)
	}

	cfg, err := apiConfig(apiCommand(t, nil))
	if err != nil {
		t.Fatalf("apiConfig() error: %v", err)
	}
	if cfg.Proxy != "http://stored.example.com:8080" {
		t.Errorf("Proxy = %q, want the stored value", cfg.Proxy)
	}
}

// And the flag still wins over a stored value.
func TestAPIConfigFlagBeatsStoredSettings(t *testing.T) {
	configHome(t)
	if err := runConfigSet(nil, []string{"proxy", "http://stored.example.com:8080"}); err != nil {
		t.Fatal(err)
	}

	cfg, err := apiConfig(apiCommand(t, map[string]string{"proxy": "http://flag.example.com:8080"}))
	if err != nil {
		t.Fatalf("apiConfig() error: %v", err)
	}
	if cfg.Proxy != "http://flag.example.com:8080" {
		t.Errorf("Proxy = %q, want the flag value", cfg.Proxy)
	}
}

// A stored CA that cannot be read fails the command, naming the path — the same error the
// flag produces, since nothing about the source changes the file being unreadable. The
// path is resolved here and read when the client is built, so both stages take part.
func TestAPIConfigStoredCACertMustExist(t *testing.T) {
	configHome(t)
	missing := filepath.Join(t.TempDir(), "gone.pem")
	if err := runConfigSet(nil, []string{"ca-cert", missing}); err != nil {
		t.Fatal(err)
	}

	cfg, err := apiConfig(apiCommand(t, nil))
	if err == nil {
		_, err = scanoss.New(cfg)
	}
	if err == nil {
		t.Fatal("an unreadable stored CA was accepted, want an error")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("error %q does not name the path", err)
	}
}
