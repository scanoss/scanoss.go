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

// Package filter decides which files a scan should process. Filtering rules are
// loaded from several sources (built-in defaults, a project's scanoss.json, and
// the tree's .gitignore), merged into a single deduplicated set, and applied in
// one pass over the source tree. Skipped files are simply excluded from the
// scan; the list of them is not tracked, only counted.
//
// The package is standalone: it does not import any other scanoss package, so
// it can be consumed on its own.
package filter

// Default skip lists are the canonical Go source, ported from scanoss.py
// (src/scanoss/file_filters.py) and cross-checked against SBOM-Workbench's
// defaultBannedList. They always apply unless an SDK caller overrides them.

// DefaultMinFileSize is the minimum file size (bytes) to scan. Files smaller than
// this are skipped. Mirrors scanoss's historical 100-byte minimum.
const DefaultMinFileSize int64 = 100

// DefaultMaxFileSize is the maximum file size (bytes) to scan. 0 means unlimited,
// matching scanoss.py's default.
const DefaultMaxFileSize int64 = 0

// DefaultSkippedDirs are directory names that are skipped wholesale (the whole
// subtree is pruned).
var DefaultSkippedDirs = []string{
	"nbproject",
	"nbbuild",
	"nbdist",
	"__pycache__",
	"venv",
	"_yardoc",
	"eggs",
	"wheels",
	"htmlcov",
	"__pypackages__",
	"example",
	"examples",
	// SBOM-Workbench parity:
	"node_modules",
	"vendor",
}

// DefaultSkippedDirExts are directory-name suffixes that are skipped (e.g. a
// directory named "foo.egg-info").
var DefaultSkippedDirExts = []string{
	".egg-info",
}

// DefaultSkippedFiles are exact file names that are skipped.
var DefaultSkippedFiles = []string{
	"gradlew",
	"gradlew.bat",
	"mvnw",
	"mvnw.cmd",
	"gradle-wrapper.jar",
	"maven-wrapper.jar",
	"thumbs.db",
	"babel.config.js",
	"license.txt",
	"license.md",
	"copying.lib",
	"makefile",
}

// DefaultSkippedExts are file-name extensions (suffixes including the leading
// dot) that are skipped. Compound extensions such as ".min.js" are matched as a
// full suffix.
var DefaultSkippedExts = []string{
	".1", ".2", ".3", ".4", ".5", ".6", ".7", ".8", ".9",
	".ac", ".adoc", ".am", ".asciidoc", ".bmp", ".build", ".cfg", ".chm",
	".class", ".cmake", ".cnf", ".conf", ".config", ".contributors", ".copying",
	".crt", ".csproj", ".css", ".csv", ".dat", ".data", ".doc", ".docx", ".dtd",
	".dts", ".iws", ".c9", ".c9revisions", ".dtsi", ".dump", ".eot", ".eps",
	".geojson", ".gdoc", ".gif", ".glif", ".gmo", ".gradle", ".guess", ".hex",
	".htm", ".html", ".ico", ".iml", ".in", ".inc", ".info", ".ini", ".ipynb",
	".jpeg", ".jpg", ".json", ".jsonld", ".lock", ".log", ".m4", ".map",
	".markdown", ".md", ".md5", ".meta", ".mk", ".mxml", ".o", ".otf", ".out",
	".pbtxt", ".pdf", ".pem", ".phtml", ".plist", ".png", ".po", ".ppt", ".prefs",
	".properties", ".pyc", ".qdoc", ".result", ".rgb", ".rst", ".scss", ".sha",
	".sha1", ".sha2", ".sha256", ".sln", ".spec", ".sql", ".sub", ".svg",
	".svn-base", ".tab", ".template", ".test", ".tex", ".tiff", ".toml", ".ttf",
	".txt", ".utf-8", ".vim", ".wav", ".woff", ".woff2", ".xht", ".xhtml", ".xls",
	".xlsx", ".xml", ".xpm", ".xsd", ".xul", ".yaml", ".yml", ".wfp",
	".editorconfig", ".dotcover", ".pid", ".lcov", ".egg", ".manifest", ".cache",
	".coverage", ".cover", ".gem", ".lst", ".pickle", ".pdb", ".gml", ".pot",
	".plt", ".whml", ".pom", ".smtml", ".min.js", ".mf", ".base64", ".s", ".diff",
	".patch", ".rules",
	// SBOM-Workbench parity:
	".mod", ".sum",
}

// DefaultSkippedFileEndings are file-name suffixes that are not extensions (no
// leading dot), matched case-insensitively against the whole file name. These
// correspond to scanoss.py's "file endings" entries.
var DefaultSkippedFileEndings = []string{
	"-doc",
	"changelog",
	"config",
	"copying",
	"license",
	"authors",
	"news",
	"licenses",
	"notice",
	"readme",
	"swiftdoc",
	"texidoc",
	"todo",
	"version",
	"ignore",
	"manifest",
	"sqlite",
	"sqlite3",
}
