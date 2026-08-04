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
	"github.com/scanoss/scanoss.go/pkg/filter"
	"strings"
	"testing"
)

func TestValidateSizeBounds(t *testing.T) {
	tests := []struct {
		name    string
		min     int64
		max     int64
		wantErr string // substring the message must name; empty means no error
	}{
		{"both unset", 0, 0, ""},
		{"minimum only", 100, 0, ""},
		{"maximum only", 0, 1024, ""},
		{"minimum below maximum", 100, 1024, ""},
		{"minimum equal to maximum", 1024, 1024, ""},
		{"negative minimum", -1, 0, "--min-size"},
		{"negative maximum", 0, -1, "--max-size"},
		{"minimum above maximum", 2048, 1024, "--min-size"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSizeBounds(tc.min, tc.max)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validateSizeBounds(%d, %d) = %v, want nil", tc.min, tc.max, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateSizeBounds(%d, %d) = nil, want an error", tc.min, tc.max)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not name %q", err, tc.wantErr)
			}
		})
	}
}

// A minimum above an unlimited maximum (0) is not a contradiction: 0 means "no
// upper bound", so the pair is valid however large the minimum is.
func TestValidateSizeBoundsUnlimitedMax(t *testing.T) {
	if err := validateSizeBounds(1<<40, 0); err != nil {
		t.Fatalf("validateSizeBounds(large, 0) = %v, want nil", err)
	}
}

// The collection flags only ever remove rules. A flag sitting at its default must not
// switch something back on that a profile turned off on purpose — the manifest stage
// starts from filter.Dependencies, whose BuiltinFolderRules is false so that a manifest
// under examples/ or venv/ is still found.
func TestApplyCollectFlagsOnlyRemovesRules(t *testing.T) {
	o := filter.Dependencies(nil)
	if o.BuiltinFolderRules || o.GitIgnore {
		t.Fatalf("precondition: Dependencies() should start with both off, got %+v", o)
	}

	// Every flag at its default: nothing the profile decided may change.
	applyCollectFlags(&o, 0, 0, false, false, true, false)
	if o.BuiltinFolderRules {
		t.Error("BuiltinFolderRules was switched on by a flag left at its default")
	}
	if o.GitIgnore {
		t.Error("GitIgnore was switched on by a flag left at its default")
	}
	if len(o.SkipDirs) == 0 {
		t.Error("the profile's own directory list was cleared by a flag left at its default")
	}

	// And the flags still do their job where the profile left the rule on.
	s := filter.Scanning(nil)
	applyCollectFlags(&s, 0, 0, true, true, false, true)
	if s.BuiltinFileRules || s.BuiltinFolderRules || s.GitIgnore {
		t.Errorf("--all-extensions/--all-folders/--gitignore=false must switch their rules off, got %+v", s)
	}
	if !s.IncludeHidden {
		t.Error("--all-hidden must switch IncludeHidden on")
	}

	// --all-folders has to reach a profile's own directory list too, not only the built-in
	// switch. Dependencies keeps its rules in SkipDirs, so a caller asking for every folder
	// would otherwise still lose a manifest under dist/.
	d := filter.Dependencies(nil)
	if len(d.SkipDirs) == 0 || len(d.SkipDirExts) == 0 {
		t.Fatal("precondition: Dependencies() should carry its own directory lists")
	}
	applyCollectFlags(&d, 0, 0, false, true, true, false)
	if len(d.SkipDirs) != 0 || len(d.SkipDirExts) != 0 {
		t.Errorf("--all-folders must clear the profile's directory rules, got SkipDirs=%v SkipDirExts=%v",
			d.SkipDirs, d.SkipDirExts)
	}
}
