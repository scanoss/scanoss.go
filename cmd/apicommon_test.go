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
	"encoding/pem"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
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

func TestAPIHTTPClientDefaults(t *testing.T) {
	client, err := newHTTPClient(apiCommand(t, nil))
	if err != nil {
		t.Fatalf("newHTTPClient() error: %v", err)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport = %T, want *http.Transport", client.Transport)
	}
	// No flags must not cost the environment's proxy.
	if transport.Proxy == nil {
		t.Error("the default client has no proxy resolver; HTTP_PROXY would be ignored")
	}
}

func TestAPIHTTPClientProxy(t *testing.T) {
	const proxy = "http://proxy.example.com:8080"

	client, err := newHTTPClient(apiCommand(t, map[string]string{"proxy": proxy}))
	if err != nil {
		t.Fatalf("newHTTPClient() error: %v", err)
	}
	req, err := http.NewRequest(http.MethodGet, "https://api.scanoss.com/", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.Transport.(*http.Transport).Proxy(req)
	if err != nil {
		t.Fatalf("resolving the proxy: %v", err)
	}
	if got == nil || got.String() != proxy {
		t.Errorf("proxy = %v, want %q", got, proxy)
	}
}

// A bad flag value must stop the command rather than reach the network.
func TestAPIHTTPClientRejectsBadFlags(t *testing.T) {
	for name, flags := range map[string]map[string]string{
		"proxy without a scheme": {"proxy": "proxy.example.com:8080"},
		"missing CA file":        {"ca-cert": filepath.Join(t.TempDir(), "nope.pem")},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := newHTTPClient(apiCommand(t, flags)); err == nil {
				t.Errorf("newHTTPClient(%v) succeeded, want an error", flags)
			}
		})
	}
}

func TestAPIHTTPClientCACert(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	caPath := filepath.Join(t.TempDir(), "ca.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(caPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	client, err := newHTTPClient(apiCommand(t, map[string]string{"ca-cert": caPath}))
	if err != nil {
		t.Fatalf("newHTTPClient() error: %v", err)
	}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("request with the CA configured: %v", err)
	}
	defer resp.Body.Close()
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
		if _, err := newHTTPClient(apiCommand(t, map[string]string{
			"ca-cert":            caPath,
			"ignore-cert-errors": "true",
		})); err != nil {
			t.Fatalf("newHTTPClient() error: %v", err)
		}
		out := logged.String()
		if !strings.Contains(out, "ignoring TLS certificate errors") {
			t.Errorf("the insecure warning is missing:\n%s", out)
		}
		if !strings.Contains(out, "--ca-cert has no effect") {
			t.Errorf("the ca-cert warning is missing:\n%s", out)
		}
	})

	// A file that holds no certificate is normally an error; with verification off
	// it is never read, so the command must still run.
	t.Run("insecure alone does not warn about the CA", func(t *testing.T) {
		logged := captureWarnings(t)
		if _, err := newHTTPClient(apiCommand(t, map[string]string{"ignore-cert-errors": "true"})); err != nil {
			t.Fatalf("newHTTPClient() error: %v", err)
		}
		if strings.Contains(logged.String(), "--ca-cert") {
			t.Errorf("warned about --ca-cert when it was not passed:\n%s", logged.String())
		}
	})
}
