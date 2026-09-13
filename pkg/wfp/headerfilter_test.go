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

package wfp

import "testing"

// Every expected offset here was taken from scanoss.py's HeaderFilter on the same input. The
// two clients must agree: the same source fingerprinted by each has to produce the same WFP, or
// it would match differently depending on which client scanned it.
func TestHeaderOffsetMatchesReferenceImplementation(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		contents string
		want     int
	}{
		{
			name: "go: license block, package and import group",
			path: "a.go",
			contents: "// SPDX-License-Identifier: MIT\n" +
				"// Copyright (c) 2026, SCANOSS\n" +
				"//\n" +
				"// Permission is hereby granted, free of charge.\n" +
				"\n" +
				"package wfp\n" +
				"\n" +
				"import (\n\t\"fmt\"\n\t\"os\"\n)\n" +
				"\n" +
				"func main() {\n\tfmt.Println(os.Args)\n}\n",
			want: 12,
		},
		{
			name: "python: shebang, docstring licence and imports",
			path: "b.py",
			contents: "#!/usr/bin/env python3\n" +
				"\"\"\"\nCopyright (c) 2025 SCANOSS\nLicensed under MIT.\n\"\"\"\n" +
				"\n" +
				"import os\nfrom typing import Optional\n" +
				"\n" +
				"def main():\n    print(os.getcwd())\n",
			want: 9,
		},
		{
			name: "c: block licence, header guard and includes",
			path: "c.c",
			contents: "/*\n * Copyright 2020 Example Corp\n * Licensed under the Apache License, Version 2.0\n */\n" +
				"#ifndef FOO_H\n#define FOO_H\n" +
				"\n" +
				"#include <stdio.h>\n#include <stdlib.h>\n" +
				"\n" +
				"int main(void) {\n    printf(\"hi\");\n    return 0;\n}\n",
			want: 10,
		},
		{
			name: "javascript: no licence, esm and require imports",
			path: "d.js",
			contents: "// no license here at all\n" +
				"import React from 'react';\n" +
				"const lodash = require('lodash');\n" +
				"\n" +
				"export function App() {\n  return null;\n}\n",
			want: 4,
		},
		{
			name: "typescript: type-only and named imports",
			path: "e.ts",
			contents: "/* MIT License */\n" +
				"import { a, b } from './x';\n" +
				"import type { C } from './y';\n" +
				"\n" +
				"export class Thing {}\n",
			want: 4,
		},
		{
			name:     "rust: use and mod",
			path:     "f.rs",
			contents: "//! Crate docs\n// Copyright holders\nuse std::fmt;\nmod inner;\n\nfn main() {}\n",
			want:     5,
		},
		{
			name:     "ruby: require and require_relative",
			path:     "g.rb",
			contents: "# frozen_string_literal: true\n# Copyright (c) 2024\nrequire 'json'\nrequire_relative 'other'\n\nclass Foo\nend\n",
			want:     5,
		},
		{
			name: "lua: block comment licence and require",
			path: "k.lua",
			contents: "--[[\nCopyright (c) 2024 Someone\nMIT license\n]]\n" +
				"local json = require(\"json\")\n\nfunction f() end\n",
			want: 1,
		},
		{
			// An unmapped extension switches the filter off: dropping lines by guesswork would
			// corrupt the fingerprint of a file whose comment syntax is unknown.
			name:     "unknown extension is left whole",
			path:     "h.txt",
			contents: "Copyright notice\nimport os\nreal content\n",
			want:     0,
		},
		{
			// Nothing but preamble means there is no implementation to point at, so nothing is
			// dropped — a fingerprint of nothing is worse than one of a header.
			name:     "a file that is all comments is left whole",
			path:     "i.py",
			contents: "# All comments, no code\n# Copyright (c) 2025\n",
			want:     0,
		},
		{
			name:     "php: the opening tag is code",
			path:     "l.php",
			contents: "<?php\n/* Copyright 2023, BSD license */\nnamespace App;\nuse App\\Thing;\n\nclass Foo {}\n",
			want:     0,
		},
		{
			name:     "go: code on the first line",
			path:     "j.go",
			contents: "package main\n\nfunc main() {}\n",
			want:     2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := headerOffset(tt.path, tt.contents, 0); got != tt.want {
				t.Errorf("headerOffset = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestHeaderOffsetRespectsTheLimit(t *testing.T) {
	// The go fixture above reports 12 unlimited.
	contents := "// SPDX-License-Identifier: MIT\n" +
		"// Copyright (c) 2026, SCANOSS\n" +
		"//\n" +
		"// Permission is hereby granted, free of charge.\n" +
		"\n" +
		"package wfp\n" +
		"\n" +
		"import (\n\t\"fmt\"\n\t\"os\"\n)\n" +
		"\n" +
		"func main() {\n\tfmt.Println(os.Args)\n}\n"

	if got := headerOffset("a.go", contents, 5); got != 5 {
		t.Errorf("headerOffset with limit 5 = %d, want 5", got)
	}
	// A limit above what was found does not inflate it.
	if got := headerOffset("a.go", contents, 100); got != 12 {
		t.Errorf("headerOffset with limit 100 = %d, want 12", got)
	}
	// Zero and negative mean no cap.
	for _, limit := range []int{0, -1} {
		if got := headerOffset("a.go", contents, limit); got != 12 {
			t.Errorf("headerOffset with limit %d = %d, want 12", limit, got)
		}
	}
}

func TestHeaderOffsetEmptyInput(t *testing.T) {
	if got := headerOffset("a.go", "", 0); got != 0 {
		t.Errorf("empty contents = %d, want 0", got)
	}
	if got := headerOffset("", "package main\n\nfunc main() {}\n", 0); got != 0 {
		t.Errorf("empty path = %d, want 0", got)
	}
}

// CRLF must not change where the code starts: the same file checked out on Windows has to
// fingerprint the same.
func TestHeaderOffsetHandlesCRLF(t *testing.T) {
	unix := "// Copyright (c) 2026\n\npackage main\n\nfunc main() {}\n"
	dos := "// Copyright (c) 2026\r\n\r\npackage main\r\n\r\nfunc main() {}\r\n"

	// 4 is what scanoss.py reports for both.
	const want = 4
	if got := headerOffset("a.go", unix, 0); got != want {
		t.Errorf("LF offset = %d, want %d", got, want)
	}
	if got := headerOffset("a.go", dos, 0); got != want {
		t.Errorf("CRLF offset = %d, want %d", got, want)
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := map[string]string{
		"a.go": "go", "a.PY": "python", "a.Go": "go",
		"dir/sub/a.tsx": "typescript", "a.m": "cpp", "a.sh": "python",
		"a.unknown": "", "noextension": "", "a.": "",
	}
	for path, want := range tests {
		t.Run(path, func(t *testing.T) {
			if got := detectLanguage(path); got != want {
				t.Errorf("detectLanguage(%q) = %q, want %q", path, got, want)
			}
		})
	}
}

// A form feed is a section separator in GNU-style sources, and it is where this port
// deliberately diverges from scanoss.py.
//
// Python's str.splitlines() breaks on \f, \v, \x1c-\x1e, \x85, U+2028 and U+2029 as well as
// \n, so the reference implementation counts more lines than the file has newlines and reports
// an offset that many lines too high. WFP minutiae are numbered by \n alone — py's own
// winnowing counts `if c == ASCII_LF` — so that offset lands in the wrong coordinate system: it
// strips a line of real code per form feed before the code starts, and emits a start_line the
// server reads as a \n-based line number.
//
// This counts by \n, which is what the marker has to mean. On 5469 real source files across the
// commissioning projects the two disagree on 44 of them (0.8%), all GNU-style.
func TestHeaderOffsetCountsNewlinesNotUnicodeLineBreaks(t *testing.T) {
	// A licence block, a form feed as the section separator, then code.
	contents := "/* Copyright (c) 2026 Example\n" +
		"   Licensed under the GNU General Public License.  */\n" +
		"\f\n" +
		"#ifndef _LIBC\n" +
		"int main(void) { return 0; }\n"

	// The first implementation line is 4 by newline counting ("#ifndef _LIBC" is not a header
	// guard, so not an import), which makes the offset 3. Python's splitlines() sees the form
	// feed as its own break and reports 4.
	if got, want := headerOffset("a.c", contents, 0), 3; got != want {
		t.Errorf("headerOffset = %d, want %d (newline-based; scanoss.py reports %d)", got, want, want+1)
	}

	// Every other Unicode line boundary Python breaks on is treated the same way here: as
	// ordinary content, not as a line break.
	for name, sep := range map[string]string{
		"vertical tab":        "\v",
		"file separator":      "\x1c",
		"group separator":     "\x1d",
		"record separator":    "\x1e",
		"next line":           "\u0085",
		"line separator":      "\u2028",
		"paragraph separator": "\u2029",
	} {
		t.Run(name, func(t *testing.T) {
			c := "// Copyright (c) 2026\n" + sep + "\nint x = 1;\n"
			// Lines: 1 = comment, 2 = the separator alone, 3 = code. Offset 2.
			if got, want := headerOffset("a.c", c, 0), 2; got != want {
				t.Errorf("headerOffset = %d, want %d", got, want)
			}
		})
	}
}
