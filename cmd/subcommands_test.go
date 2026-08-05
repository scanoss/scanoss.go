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
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func findCmd(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

// Every service command exposes the expected subcommands.
func TestServiceSubcommandsRegistered(t *testing.T) {
	cases := map[string][]string{
		"vulnerabilities": {"components", "cpes"},
		"cryptography":    {"algorithms", "algorithms-range", "versions-range", "hints", "hints-range"},
		"geoprovenance":   {"origin", "countries"},
		"licenses":        {"declared", "attribution", "evidence"},
		"copyright":       {"evidence", "holders"},
		"components":      {"search", "versions", "status"},
	}
	for parent, subs := range cases {
		pc := findCmd(rootCmd, parent)
		if pc == nil {
			t.Errorf("missing command %q", parent)
			continue
		}
		for _, s := range subs {
			if findCmd(pc, s) == nil {
				t.Errorf("%s: missing subcommand %q", parent, s)
			}
		}
	}
}

// A PURL-list subcommand inherits the shared flags from its parent.
func TestSubcommandInheritsSharedFlags(t *testing.T) {
	hints := findCmd(findCmd(rootCmd, "cryptography"), "hints")
	if hints == nil {
		t.Fatal("cryptography hints subcommand not found")
	}
	for _, f := range []string{"purl", "input", "chunk-size", "workers", "api-url", "api-key", "output"} {
		if hints.InheritedFlags().Lookup(f) == nil {
			t.Errorf("cryptography hints: missing inherited flag --%s", f)
		}
	}
}

// Each subcommand routes to its v3 endpoint.
func TestSubcommandRouting(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = w.Write([]byte(`{"components":[]}`))
	}))
	defer srv.Close()

	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
	out := filepath.Join(t.TempDir(), "out.json")

	exec := func(args ...string) error {
		rootCmd.SetArgs(append(args, "--api-url", srv.URL, "--output", out))
		return rootCmd.Execute()
	}

	// PURL-list subcommands (POST). Bare-parent defaults are covered too.
	purlCases := []struct {
		args []string
		want string
	}{
		{[]string{"vulnerabilities"}, "/v3/vulnerabilities/vulnerabilities"}, // default op
		{[]string{"vulnerabilities", "components"}, "/v3/vulnerabilities/vulnerabilities"},
		{[]string{"vulnerabilities", "cpes"}, "/v3/vulnerabilities/cpes"},
		{[]string{"cryptography"}, "/v3/cryptography/algorithms"}, // default op
		{[]string{"cryptography", "algorithms"}, "/v3/cryptography/algorithms"},
		{[]string{"cryptography", "algorithms-range"}, "/v3/cryptography/algorithms/range"},
		{[]string{"cryptography", "versions-range"}, "/v3/cryptography/algorithms/versions/range"},
		{[]string{"cryptography", "hints"}, "/v3/cryptography/hints"},
		{[]string{"cryptography", "hints-range"}, "/v3/cryptography/hints/range"},
		{[]string{"geoprovenance"}, "/v3/geoprovenance/origin"}, // default op
		{[]string{"geoprovenance", "countries"}, "/v3/geoprovenance/countries"},
		{[]string{"licenses"}, "/v3/licenses"}, // default op (declared)
		{[]string{"licenses", "declared"}, "/v3/licenses"},
		{[]string{"licenses", "attribution"}, "/v3/license/attribution"},
		{[]string{"licenses", "evidence"}, "/v3/license/evidence"},
		{[]string{"copyright"}, "/v3/copyright/evidence"}, // default op
		{[]string{"copyright", "holders"}, "/v3/copyright/holders"},
		{[]string{"components", "status"}, "/v3/components/status"},
	}
	for _, tc := range purlCases {
		gotPath = ""
		if err := exec(append(tc.args, "--purl", "pkg:test")...); err != nil {
			t.Fatalf("%v: %v", tc.args, err)
		}
		if gotPath != tc.want {
			t.Errorf("%v: path=%q want %q", tc.args, gotPath, tc.want)
		}
	}

	// components search (GET) — default op and explicit subcommand.
	gotPath, gotQuery = "", ""
	if err := exec("components", "--vendor", "angular"); err != nil {
		t.Fatalf("components (search default): %v", err)
	}
	if gotPath != "/v3/components/search" {
		t.Errorf("components default: path=%q want /v3/components/search", gotPath)
	}
	if !strings.Contains(gotQuery, "vendor=angular") {
		t.Errorf("components search: query=%q want vendor=angular", gotQuery)
	}

	// components versions (GET) — purl + limit query params.
	gotPath, gotQuery = "", ""
	if err := exec("components", "versions", "--purl", "pkg:a", "--limit", "5"); err != nil {
		t.Fatalf("components versions: %v", err)
	}
	if gotPath != "/v3/components/versions" {
		t.Errorf("components versions: path=%q want /v3/components/versions", gotPath)
	}
	if !strings.Contains(gotQuery, "limit=5") {
		t.Errorf("components versions: query=%q want limit=5", gotQuery)
	}

	// components releases (GET) — purl + requirement + limit + offset query params.
	gotPath, gotQuery = "", ""
	if err := exec("components", "releases", "--purl", "pkg:a", "--requirement", ">=1.0.0", "--limit", "10", "--offset", "20"); err != nil {
		t.Fatalf("components releases: %v", err)
	}
	if gotPath != "/v3/components/releases" {
		t.Errorf("components releases: path=%q want /v3/components/releases", gotPath)
	}
	for _, want := range []string{"limit=10", "offset=20", "requirement="} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("components releases: query=%q missing %q", gotQuery, want)
		}
	}
}
