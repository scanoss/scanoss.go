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
	"os"
	"testing"
)

// TestMain isolates the settings sources for the whole package.
//
// The commands now resolve --api-url/--api-key through $SCANOSS_API_* and
// ~/.scanoss/settings.json. Without this, a developer with SCANOSS_API_KEY exported
// (or a stored config) would have their own credential satisfy the no-key guard, and
// tests that assert the guard fires would pass or fail depending on the machine.
// HOME is redirected as well as the variables unset, so a real settings file on the
// developer's box cannot reach the tests either.
func TestMain(m *testing.M) {
	for _, name := range []string{"SCANOSS_API_KEY", "SCANOSS_API_URL"} {
		if err := os.Unsetenv(name); err != nil {
			fmt.Fprintf(os.Stderr, "unsetting %s: %v\n", name, err)
			os.Exit(1)
		}
	}

	home, err := os.MkdirTemp("", "scanoss-cmd-home")
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating a temporary home: %v\n", err)
		os.Exit(1)
	}
	// os.UserHomeDir reads HOME on unix and USERPROFILE on Windows.
	for name, value := range map[string]string{"HOME": home, "USERPROFILE": home} {
		if err := os.Setenv(name, value); err != nil {
			fmt.Fprintf(os.Stderr, "setting %s: %v\n", name, err)
			os.Exit(1)
		}
	}

	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}
