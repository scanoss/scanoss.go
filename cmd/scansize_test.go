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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// scan exposes both size bounds, each defaulting to "no bound".
func TestScanSizeFlags(t *testing.T) {
	sc := findCmd(rootCmd, "scan")
	if sc == nil {
		t.Fatal("scan command not found")
	}
	for _, name := range []string{"min-size", "max-size"} {
		f := sc.Flags().Lookup(name)
		if f == nil {
			t.Fatalf("scan: missing flag --%s", name)
		}
		if f.DefValue != "0" {
			t.Errorf("scan --%s default = %q, want 0", name, f.DefValue)
		}
	}
	if u := sc.Flags().Lookup("min-size").Usage; !strings.Contains(u, "no minimum") {
		t.Errorf("--min-size usage %q should say what 0 means", u)
	}
}

// An impossible or malformed size pair is refused before the scan starts — no
// file is read and no request reaches the API.
func TestScanRejectsInvalidSizeBounds(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(strings.Repeat("a", 200)), 0o644); err != nil {
		t.Fatal(err)
	}

	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true

	// Both bounds are passed on every run: cobra keeps flag values between
	// Execute calls in one process, so a case that omitted one would inherit the
	// previous case's value.
	tests := []struct {
		name     string
		min, max string
		want     string
	}{
		{"negative minimum", "-1", "0", "--min-size"},
		{"negative maximum", "0", "-1", "--max-size"},
		{"minimum above maximum", "2048", "1024", "--min-size"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rootCmd.SetArgs([]string{
				"scan", root,
				"--min-size", tc.min, "--max-size", tc.max,
				"--api-url", srv.URL, "--api-key", "test",
			})
			err := rootCmd.Execute()
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not name %q", err, tc.want)
			}
		})
	}

	if got := hits.Load(); got != 0 {
		t.Errorf("API was called %d times; validation must happen before any request", got)
	}
}
