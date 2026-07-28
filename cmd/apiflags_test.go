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
)

// TestAPIFlagsAreResolvedNotRead guards the precedence chain against the one way it
// can be quietly bypassed: a command reading --api-url/--api-key straight off the
// flag set. That code compiles, passes review, and silently ignores both
// $SCANOSS_API_* and ~/.scanoss/settings.json for that command only.
//
// Every read must go through cliconfig.ResolveAPI. This walks the package's own
// sources rather than trusting the convention to hold.
func TestAPIFlagsAreResolvedNotRead(t *testing.T) {
	// Reading the flags is legitimate inside the config command itself, which
	// reports and edits the stored settings rather than consuming them.
	allowed := map[string]bool{}

	forbidden := []string{
		`GetString("api-url")`,
		`GetString("api-key")`,
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if allowed[name] {
			continue
		}
		source, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		for _, pattern := range forbidden {
			if strings.Contains(string(source), pattern) {
				t.Errorf("%s calls %s directly; use cliconfig.ResolveAPI(cmd.Flags()) so the "+
					"environment and ~/.scanoss/settings.json are honoured", name, pattern)
			}
		}
	}
}
