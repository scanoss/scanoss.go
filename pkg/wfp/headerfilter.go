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

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// headerOffset finds where a source file stops being preamble and starts being code: license
// headers, documentation comments and imports are preamble, and the fingerprint of a file is a
// better match signal without them — two unrelated files sharing a common licence header should
// not look alike.
//
// It returns the number of leading lines to drop, or 0 when there is nothing to drop, the
// language is not one it knows, or the whole file is preamble.
//
// This is a port of scanoss.py's HeaderFilter, and deliberately a faithful one: the two clients
// must produce the same WFP for the same file, or the same source scanned by each would match
// differently.
func headerOffset(path, contents string, maxLines int) int {
	if contents == "" || path == "" {
		return 0
	}
	language := detectLanguage(path)
	if language == "" {
		return 0
	}
	// Split without keeping the line terminators. The Python original keeps them and relies on
	// its "$" matching just before a trailing newline; Go's does not, so the newline is dropped
	// here instead and the patterns below behave the same in both.
	//
	// Lines are newline-delimited, and only newline-delimited. See splitLines.
	lines := splitLines(contents)
	if len(lines) == 0 {
		return 0
	}

	start := firstImplementationLine(lines, language)
	if start == 0 {
		return 0 // no implementation found: the file is preamble all the way down
	}
	offset := start - 1
	if maxLines > 0 && maxLines < offset {
		return maxLines
	}
	return offset
}

// splitLines splits on newlines, tolerating CRLF, and drops a trailing empty line so a file
// ending in a newline does not report one line more than it has.
//
// It splits on "\n" and nothing else, which is a deliberate divergence from the reference
// implementation. Python's str.splitlines() also breaks on \v, \f, \x1c-\x1e, \x85, U+2028 and
// U+2029, so scanoss.py counts a form feed — the GNU section separator — as a line of its own and
// reports an offset one line too high for each one before the code starts. WFP minutiae are
// numbered by \n alone (py's own winnowing counts `if c == ASCII_LF`), so that offset lands in
// the wrong coordinate system and strips a line of real code. The marker has to mean a \n-based
// line number, so that is what is counted here.
func splitLines(contents string) []string {
	lines := strings.Split(contents, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}

// trimSpace strips leading and trailing whitespace the way Python's str.strip() does.
//
// It matters because a line is classified as blank when nothing is left of it. Python counts the
// separator controls \x1c-\x1f as whitespace and Go's unicode.IsSpace does not, so a line
// holding nothing but one of them would read as the first line of code here and as a blank line
// there — and the offsets would part ways.
func trimSpace(s string) string {
	return strings.TrimFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f)
	})
}

// licenseHeaderMaxLines bounds how far into a file a licence block is still believable. Past it,
// a comment mentioning "copyright" is much more likely to be ordinary prose than a header.
const licenseHeaderMaxLines = 50

// firstImplementationLine returns the 1-indexed line where the code proper begins, or 0 if the
// file never gets there. It stops at the first such line rather than classifying the whole file.
func firstImplementationLine(lines []string, language string) int {
	var (
		inMultilineComment bool
		inLicenseSection   bool
		insideImportBlock  bool
	)
	comments := commentPatternsFor(language)
	imports := importPatterns[language]

	for i, line := range lines {
		stripped := trimSpace(line)

		// A shebang counts as preamble only on the first line; anywhere else "#!" is just a
		// comment, and is classified as one below.
		if (i == 0 && strings.HasPrefix(stripped, "#!")) || stripped == "" {
			continue
		}

		isComment, stillInMultiline := classifyComment(line, inMultilineComment, comments)
		inMultilineComment = stillInMultiline
		if isComment {
			switch {
			case isLicenseHeader(line):
				inLicenseSection = true
			case inLicenseSection && i+1 < licenseHeaderMaxLines:
				// Still inside the licence block: its later lines need not each name a
				// licence keyword.
			default:
				inLicenseSection = false
			}
			continue
		}
		inLicenseSection = false

		// A multi-line import block ends at its closing bracket; everything between is part
		// of it, whether or not those lines look like imports on their own.
		if insideImportBlock {
			if strings.Contains(stripped, ")") || strings.Contains(stripped, "}") {
				insideImportBlock = false
			}
			continue
		}

		if matchesAny(imports, line) {
			// An import that opens a bracket it does not close starts a block.
			if (strings.Contains(stripped, "(") && !strings.Contains(stripped, ")")) ||
				(strings.Contains(stripped, "{") && !strings.Contains(stripped, "}")) {
				insideImportBlock = true
			}
			continue
		}

		return i + 1
	}
	return 0
}

// isLicenseHeader reports whether a comment line reads like part of a licence block.
func isLicenseHeader(line string) bool {
	lower := strings.ToLower(line)
	for _, keyword := range licenseKeywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

// classifyComment reports whether a line is a comment, and whether a multi-line comment is still
// open after it.
func classifyComment(line string, inMultiline bool, p *commentPatterns) (isComment, stillInMultiline bool) {
	if p == nil {
		return false, inMultiline
	}
	if inMultiline {
		if p.multiEnd != nil && p.multiEnd.MatchString(line) {
			return true, false
		}
		if p.docStringEnd != nil && p.docStringEnd.MatchString(line) {
			return true, false
		}
		return true, true
	}
	if p.singleLine != nil && p.singleLine.MatchString(line) {
		return true, false
	}
	if p.multiSingle != nil && p.multiSingle.MatchString(line) {
		return true, false
	}
	if p.multiStart != nil && p.multiStart.MatchString(line) {
		// A block that also closes on its own line leaves nothing open.
		if p.multiEnd != nil && p.multiEnd.MatchString(line) {
			return true, false
		}
		return true, true
	}
	// A Python docstring: two delimiters on one line is a complete one, a single delimiter
	// opens a block.
	if p.docStringStart != nil && strings.Contains(line, `"""`) {
		switch strings.Count(line, `"""`) {
		case 2:
			return true, false
		case 1:
			return true, true
		}
	}
	return false, inMultiline
}

func matchesAny(patterns []*regexp.Regexp, line string) bool {
	for _, re := range patterns {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}

// detectLanguage maps a file extension to the language whose comment and import shapes apply.
// An unmapped extension yields "", which switches the filter off for that file: dropping lines
// by guesswork would corrupt the fingerprint.
func detectLanguage(path string) string {
	return extLanguage[strings.ToLower(filepath.Ext(path))]
}

// commentPatterns is one language family's comment shapes. A nil field means the family has no
// such construct — HTML has no single-line comment, C has no docstring.
type commentPatterns struct {
	singleLine     *regexp.Regexp
	multiStart     *regexp.Regexp
	multiEnd       *regexp.Regexp
	multiSingle    *regexp.Regexp
	docStringStart *regexp.Regexp
	docStringEnd   *regexp.Regexp
}

var (
	cStyleComments = &commentPatterns{
		singleLine:  regexp.MustCompile(`^\s*//.*$`),
		multiStart:  regexp.MustCompile(`^\s*/\*`),
		multiEnd:    regexp.MustCompile(`\*/\s*$`),
		multiSingle: regexp.MustCompile(`^\s*/\*.*\*/\s*$`),
	}
	pythonStyleComments = &commentPatterns{
		singleLine:     regexp.MustCompile(`^\s*#.*$`),
		docStringStart: regexp.MustCompile(`^\s*"""`),
		docStringEnd:   regexp.MustCompile(`"""\s*$`),
	}
	luaStyleComments = &commentPatterns{
		singleLine: regexp.MustCompile(`^\s*--.*$`),
		multiStart: regexp.MustCompile(`^\s*--\[\[`),
		multiEnd:   regexp.MustCompile(`\]\]\s*$`),
	}
)

// commentPatternsFor picks the comment family for a language. Anything unlisted falls back to
// C style, as the reference implementation does.
func commentPatternsFor(language string) *commentPatterns {
	switch language {
	case "cpp", "java", "kotlin", "scala", "javascript", "typescript",
		"go", "rust", "csharp", "php", "swift", "dart":
		return cStyleComments
	case "python", "ruby", "perl", "r":
		return pythonStyleComments
	case "lua", "haskell":
		return luaStyleComments
	}
	return cStyleComments
}

// licenseKeywords are the words that mark a comment as part of a licence or copyright block.
// Lowercase: the line is folded before comparison.
var licenseKeywords = []string{
	"copyright", "license", "licensed", "all rights reserved",
	"permission", "redistribution", "warranty", "liability",
	"apache", "mit", "gpl", "bsd", "mozilla", "author:",
	"spdx-license", "contributors", "licensee",
}

// extLanguage maps a lowercase file extension to its language. Shell scripts borrow Python's
// comment style; they have no import patterns, so "source"/"." lines are not filtered.
var extLanguage = map[string]string{
	".py": "python",
	".js": "javascript", ".mjs": "javascript", ".cjs": "javascript", ".jsx": "javascript",
	".ts": "typescript", ".tsx": "typescript",
	".java": "java",
	".kt":   "kotlin", ".kts": "kotlin",
	".scala": "scala", ".sc": "scala",
	".go":  "go",
	".rs":  "rust",
	".cpp": "cpp", ".cc": "cpp", ".cxx": "cpp", ".c": "cpp",
	".h": "cpp", ".hpp": "cpp", ".hxx": "cpp",
	".cs":    "csharp",
	".php":   "php",
	".swift": "swift",
	".rb":    "ruby",
	".pl":    "perl", ".pm": "perl",
	".r":    "r",
	".lua":  "lua",
	".dart": "dart",
	".hs":   "haskell",
	".ex":   "elixir", ".exs": "elixir",
	".clj": "clojure", ".cljs": "clojure",
	".m": "cpp", ".mm": "cpp", // Objective-C / Objective-C++
	".sh": "python", ".bash": "python", ".zsh": "python", ".fish": "python",
}

// importPatterns are the import/include shapes per language, in the order the reference
// implementation lists them.
var importPatterns = map[string][]*regexp.Regexp{
	"python": mustCompileAll(
		`^\s*import\s+`,
		`^\s*from\s+.*\s+import\s+`,
	),
	"javascript": mustCompileAll(
		`^\s*import\s+.*\s+from\s+`,
		`^\s*import\s+["']`,
		`^\s*import\s+type\s+`,
		`^\s*export\s+\*\s+from\s+`,
		`^\s*export\s+\{.*\}\s+from\s+`,
		`^\s*const\s+.*\s*=\s*require\(`,
		`^\s*var\s+.*\s*=\s*require\(`,
		`^\s*let\s+.*\s*=\s*require\(`,
	),
	"typescript": mustCompileAll(
		`^\s*import\s+`,
		`^\s*export\s+.*\s+from\s+`,
		`^\s*import\s+type\s+`,
		`^\s*import\s+\{.*\}\s+from\s+`,
	),
	"java":   mustCompileAll(`^\s*import\s+`, `^\s*package\s+`),
	"kotlin": mustCompileAll(`^\s*import\s+`, `^\s*package\s+`),
	"scala":  mustCompileAll(`^\s*import\s+`, `^\s*package\s+`),
	"go": mustCompileAll(
		`^\s*import\s+\(`,
		`^\s*import\s+"`,
		`^\s*package\s+`,
		`^\s*"[^"]*"\s*$`, // an import inside an import ( ... ) block
		`^\s*[a-zA-Z_][a-zA-Z0-9_]*\s+"[^"]*"\s*$`, // aliased import
		`^\s*_\s+"[^"]*"\s*$`,                      // blank import
	),
	"rust": mustCompileAll(`^\s*use\s+`, `^\s*extern\s+crate\s+`, `^\s*mod\s+`),
	"cpp": mustCompileAll(
		`^\s*#include\s+`,
		`^\s*#pragma\s+`,
		`^\s*#ifndef\s+.*_H.*`, // header guard
		`^\s*#define\s+.*_H.*`, // header guard
		`^\s*#endif\s+(//.*)?\s*$`,
	),
	"csharp": mustCompileAll(`^\s*using\s+`, `^\s*namespace\s+`),
	"php": mustCompileAll(
		`^\s*use\s+`,
		`^\s*require\s+`,
		`^\s*require_once\s+`,
		`^\s*include\s+`,
		`^\s*include_once\s+`,
		`^\s*namespace\s+`,
	),
	"swift":   mustCompileAll(`^\s*import\s+`),
	"ruby":    mustCompileAll(`^\s*require\s+`, `^\s*require_relative\s+`, `^\s*load\s+`),
	"perl":    mustCompileAll(`^\s*use\s+`, `^\s*require\s+`),
	"r":       mustCompileAll(`^\s*library\(`, `^\s*require\(`, `^\s*source\(`),
	"lua":     mustCompileAll(`^\s*require\s+`, `^\s*local\s+.*\s*=\s*require\(`),
	"dart":    mustCompileAll(`^\s*import\s+`, `^\s*export\s+`, `^\s*part\s+`),
	"haskell": mustCompileAll(`^\s*import\s+`, `^\s*module\s+`),
	"elixir": mustCompileAll(
		`^\s*import\s+`, `^\s*alias\s+`, `^\s*require\s+`, `^\s*use\s+`,
	),
	"clojure": mustCompileAll(`^\s*\(\s*ns\s+`, `^\s*\(\s*require\s+`, `^\s*\(\s*import\s+`),
}

func mustCompileAll(exprs ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(exprs))
	for _, e := range exprs {
		out = append(out, regexp.MustCompile(e))
	}
	return out
}
