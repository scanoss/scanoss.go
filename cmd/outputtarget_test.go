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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestValidateOutputTarget(t *testing.T) {
	dir := t.TempDir()
	regularFile := filepath.Join(dir, "a-file")
	if err := os.WriteFile(regularFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		output string
		wantOK bool
	}{
		{"stdout, or a command with no --output", "", true},
		{"an existing directory", filepath.Join(dir, "out.json"), true},
		{"a bare filename, so the working directory", "out.json", true},
		{"a directory that does not exist", filepath.Join(dir, "nope", "out.json"), false},
		{"a regular file used as a directory", filepath.Join(regularFile, "out.json"), false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmd := newTestCommandWithOutput(c.output)
			err := validateOutputTarget(cmd)
			if c.wantOK && err != nil {
				t.Errorf("rejected a writable target: %v", err)
			}
			if !c.wantOK {
				if err == nil {
					t.Fatal("accepted a target that cannot be written to")
				}
				if !strings.Contains(err.Error(), c.output) {
					t.Errorf("the error should name the path the user gave, got: %v", err)
				}
			}
		})
	}

	// Checking must not create anything: a run that fails later should leave nothing behind,
	// not an empty file where results were expected.
	if entries, err := os.ReadDir(dir); err != nil {
		t.Fatal(err)
	} else if len(entries) != 1 {
		t.Errorf("validation created something: %v", entries)
	}
}

// The check belongs to the root's PersistentPreRunE, so it happens before the command runs rather
// than when the writer is finally opened. Pointing at an unreachable API proves the ordering: if
// the target were checked at write time, the connection error would arrive first and the scan's
// work would already be spent.
func TestOutputTargetIsCheckedBeforeTheCommandRuns(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "nope", "out.json")

	for _, argv := range [][]string{
		{"results", "019fb000-0000-0000-0000-000000000000"},
		{"licenses", "--purl", "pkg:npm/lodash"},
	} {
		full := append(argv, "--output", bad, "--api-key", "x", "--api-url", "http://127.0.0.1:1")
		t.Run(argv[0], func(t *testing.T) {
			_, _, err := runBare(t, full...)
			if err == nil {
				t.Fatal("an unwritable --output was accepted")
			}
			if !strings.Contains(err.Error(), "cannot write to") {
				t.Errorf("failed for the wrong reason, so the target was checked too late: %v", err)
			}
		})
	}
}

// newTestCommandWithOutput builds a command carrying just the --output flag, set to path.
func newTestCommandWithOutput(path string) *cobra.Command {
	c := &cobra.Command{Use: "test"}
	c.Flags().StringP("output", "o", "", "")
	if path != "" {
		_ = c.Flags().Set("output", path)
	}
	return c
}
