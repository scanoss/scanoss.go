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
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// clearFlags returns every flag on the tree to its zero state.
//
// Cobra keeps flag values on the command between Execute calls, so in a single test binary a
// command invoked with nothing would otherwise inherit whatever an earlier test passed — and a
// leftover --purl is precisely what stops these cases from reaching the guard they are about.
func clearFlags(c *cobra.Command) {
	clear := func(f *pflag.Flag) {
		if slice, ok := f.Value.(pflag.SliceValue); ok {
			_ = slice.Replace(nil) // Set() appends on a slice flag, so it cannot clear one
		} else {
			_ = f.Value.Set(f.DefValue)
		}
		f.Changed = false
	}
	c.Flags().VisitAll(clear)
	c.PersistentFlags().VisitAll(clear)
	for _, sub := range c.Commands() {
		clearFlags(sub)
	}
}

// runBare invokes a command and reports what it wrote where, plus the error it returned.
// Execute() turns a non-nil error into exit 1, so the error standing in for the exit code is the
// whole point.
//
// Flags are cleared on the way out as well as on the way in: cobra keeps them on the command
// between calls, so a value left behind here would reach whichever test runs next — which is not
// hypothetical, an --output left set made an unrelated size-bounds test fail on the wrong error.
func runBare(t *testing.T, argv ...string) (stdout, stderr string, err error) {
	t.Helper()
	clearFlags(rootCmd)
	defer clearFlags(rootCmd)

	var out, errOut bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errOut)
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
	rootCmd.SetArgs(argv)
	err = rootCmd.Execute()
	return out.String(), errOut.String(), err
}

// Every command that needs an argument has to fail without one. A command that prints help and
// succeeds is indistinguishable, to a CI step, from a scan that ran and found nothing: the exit
// code says fine and stdout carries text where the results belong.
//
// The empty-variable spelling is what makes this reachable in practice — `scanoss-cli scan $DIR`
// with DIR unset passes no argument at all, so the command sees exactly this.
func TestCommandsWithoutTheirArgumentFail(t *testing.T) {
	cases := []struct {
		argv []string
		want string
	}{
		{[]string{"scan"}, "a path to scan is required"},
		{[]string{"scan", "wfp"}, "a WFP file is required"},
		{[]string{"wfp"}, "a path to fingerprint is required"},
		{[]string{"results"}, "a scan id is required"},
		{[]string{"enrich"}, "an SBOM file to enrich is required"},
		{[]string{"sbom"}, "an input file is required"},
		{[]string{"attributions"}, "an SBOM file or --purl is required"},
		{[]string{"dependencies"}, "a path, --purl or --extract-local is required"},
		{[]string{"licenses"}, "--purl or --input is required"},
		{[]string{"vulnerabilities"}, "--purl or --input is required"},
		{[]string{"cryptography"}, "--purl or --input is required"},
		{[]string{"components", "search"}, "--search, --vendor or --component is required"},
		{[]string{"components", "versions"}, "--purl is required"},
	}

	for _, c := range cases {
		t.Run(strings.Join(c.argv, " "), func(t *testing.T) {
			stdout, stderr, err := runBare(t, c.argv...)

			if err == nil {
				t.Fatal("returned no error, so the process would exit 0 having done nothing")
			}
			if err.Error() != c.want {
				t.Errorf("error should name what is missing: got %q, want %q", err, c.want)
			}
			if stdout != "" {
				t.Errorf("nothing may reach stdout, which is where results go; got %d bytes", len(stdout))
			}
			if !strings.Contains(stderr, "Usage:") {
				t.Errorf("usage should be shown on stderr, got: %q", stderr)
			}
		})
	}
}

// A namespace was not asked to do anything, so listing what it holds is a complete answer.
func TestNamespaceCommandSucceeds(t *testing.T) {
	stdout, _, err := runBare(t, "config")
	if err != nil {
		t.Fatalf("a bare namespace command should succeed, got: %v", err)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Errorf("its help should be shown, got %q", stdout)
	}
}
