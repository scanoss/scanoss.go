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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scanoss/scanoss.go/pkg/wfp"
)

// headerSource is a C file whose first seven lines are a licence block and includes, so the
// header filter has something to drop and announces it with start_line=7.
func headerSource() string {
	var sb strings.Builder
	sb.WriteString("/*\n * Copyright (c) 2026 Example Corp\n * Licensed under the MIT license\n */\n")
	sb.WriteString("#include <stdio.h>\n#include <stdlib.h>\n\n")
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&sb, "int compute_%d(int x) { return x * %d + 7; }\n", i, i)
	}
	return sb.String()
}

// runWFPSkipHeaders runs the real wfp command over root with an explicit --skip-headers value.
// Every flag the command reads is passed: cobra keeps flag values between Execute calls in one
// process, so an omitted one would inherit the last run's.
func runWFPSkipHeaders(t *testing.T, root, skipHeaders string) string {
	t.Helper()
	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true

	out := filepath.Join(t.TempDir(), "out.wfp")
	rootCmd.SetArgs([]string{
		"wfp", root, "--output", out,
		"--min-size", "0", "--max-size", "0",
		"--all-extensions=false", "--gitignore=true",
		"--skip-headers=" + skipHeaders, "--skip-headers-limit", "0",
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("wfp: %v", err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func headerTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "header.c"), []byte(headerSource()), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// pkg/wfp filters by default, so the CLI turning the filter off has to say so: absence of an
// option no longer means off. --skip-headers=false must still yield the unfiltered WFP.
func TestWFPSkipHeadersFlagOff(t *testing.T) {
	root := headerTree(t)

	if got := runWFPSkipHeaders(t, root, "false"); strings.Contains(got, "start_line=") {
		t.Errorf("with --skip-headers=false the WFP must be unfiltered; got:\n%s", got)
	}
	// Run with the filter on last, so the value cobra keeps for later tests is the default.
	if got := runWFPSkipHeaders(t, root, "true"); !strings.Contains(got, "start_line=7\n") {
		t.Errorf("with --skip-headers the WFP should carry start_line=7; got:\n%s", got)
	}
}

// skip_headers: false in scanoss.json still turns the filter off, and still beats the flag.
func TestWFPSkipHeadersSettingsOff(t *testing.T) {
	root := headerTree(t)
	cfg := `{"settings": {"file_snippet": {"skip_headers": false}}}`
	if err := os.WriteFile(filepath.Join(root, "scanoss.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := runWFPSkipHeaders(t, root, "true"); strings.Contains(got, "start_line=") {
		t.Errorf("with skip_headers: false in scanoss.json the WFP must be unfiltered; got:\n%s", got)
	}
}

// fingerprintOptions is shared by `scan` and `wfp`; both states must reach the fingerprinter.
func TestFingerprintOptionsPassBothStates(t *testing.T) {
	root := headerTree(t)
	path := filepath.Join(root, "header.c")

	on := wfp.Files([]string{path}, 1, root, nil, fingerprintOptions(true, 0)...)
	if !strings.Contains(string(on.WFP), "start_line=7\n") {
		t.Errorf("filter on: want start_line=7; got:\n%s", on.WFP)
	}
	limited := wfp.Files([]string{path}, 1, root, nil, fingerprintOptions(true, 3)...)
	if !strings.Contains(string(limited.WFP), "start_line=3\n") {
		t.Errorf("filter on, limit 3: want start_line=3; got:\n%s", limited.WFP)
	}
	off := wfp.Files([]string{path}, 1, root, nil, fingerprintOptions(false, 10)...)
	if strings.Contains(string(off.WFP), "start_line=") {
		t.Errorf("filter off: WFP must be unfiltered; got:\n%s", off.WFP)
	}
}
