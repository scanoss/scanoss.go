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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestComponentsReleasesNoPurl proves a missing --purl is a usage error (surfaced
// before any auth/API work), not the auth banner.
func TestComponentsReleasesNoPurl(t *testing.T) {
	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true

	rootCmd.SetArgs([]string{"components", "releases"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected a usage error when --purl is missing")
	}
}

// TestComponentsReleasesUnavailable proves RELEASE_NOTES_UNAVAILABLE is handled
// gracefully: an explicit stderr notice, the JSON still written, and a zero exit.
func TestComponentsReleasesUnavailable(t *testing.T) {
	const body = `{"component":{"purl":"pkg:github/scanoss/engine","version":"5.4.7",` +
		`"info_code":"RELEASE_NOTES_UNAVAILABLE"},"releases":[],"status":{"message":"ok","status":"SUCCESS"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	var errBuf bytes.Buffer
	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(&errBuf)
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true

	out := filepath.Join(t.TempDir(), "out.json")
	rootCmd.SetArgs([]string{
		"components", "releases", "--purl", "pkg:github/scanoss/engine",
		"--requirement", "5.4.7", "--api-url", srv.URL, "--output", out,
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("RELEASE_NOTES_UNAVAILABLE should not error, got: %v", err)
	}

	if notice := errBuf.String(); !strings.Contains(notice, "no release notes available for pkg:github/scanoss/engine@5.4.7") {
		t.Errorf("stderr = %q, want the 'no release notes available' notice", notice)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	if !strings.Contains(string(data), "RELEASE_NOTES_UNAVAILABLE") {
		t.Errorf("output JSON = %q, want the info_code preserved", string(data))
	}
}
