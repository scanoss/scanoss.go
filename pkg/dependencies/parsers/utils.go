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

package parsers

import (
	"bytes"
	"net/url"
	"regexp"
	"strings"
)

// IsValidURL checks if a string is a valid URL
func IsValidURL(str string) bool {
	_, err := url.ParseRequestURI(str)
	return err == nil
}

// IsValidPath checks if a string matches a file system path structure
func IsValidPath(str string) bool {
	// Matches relative paths (./,  ../), Windows paths (C:\), and Unix paths (/)
	pathRegex := regexp.MustCompile(`^(\.{1,2}\/|[a-zA-Z]:\\|\\|\/)`)
	return pathRegex.MatchString(str)
}

// TrimComments removes inline comments from a line
func TrimComments(line string, commentChars ...string) string {
	if len(commentChars) == 0 {
		commentChars = []string{"//", "#"}
	}

	for _, char := range commentChars {
		if idx := strings.Index(line, char); idx != -1 {
			line = line[:idx]
		}
	}

	return strings.TrimSpace(line)
}

// PreprocessLine removes comments and trims whitespace
func PreprocessLine(line string) string {
	return strings.TrimSpace(TrimComments(line))
}

// SplitNamespace splits a package identifier into namespace and name
// For example: "github.com/spf13/cobra" -> ("github.com/spf13", "cobra")
func SplitNamespace(fullPath string) (namespace, name string) {
	lastSlash := strings.LastIndex(fullPath, "/")
	if lastSlash == -1 {
		return "", fullPath
	}
	return fullPath[:lastSlash], fullPath[lastSlash+1:]
}

// GetScopedPackage splits a scoped package name into namespace and name.
// The npm scope is part of the PURL namespace, so the leading "@" is preserved
// (per the PURL spec the namespace of a scoped package is the scope including
// the "@"). For example: "@angular/core" -> ("@angular", "core").
//
// Dropping the "@" produces "pkg:npm/angular/core", which the SCANOSS
// decoration services do not resolve (they index scoped packages under the
// canonical "@scope" namespace) — every scoped npm dependency would silently
// come back without vulnerabilities or licenses.
func GetScopedPackage(pkgName string) (namespace, name string) {
	if !strings.HasPrefix(pkgName, "@") {
		return "", pkgName
	}

	parts := strings.SplitN(pkgName[1:], "/", 2)
	if len(parts) == 2 {
		return "@" + parts[0], parts[1]
	}
	return "", pkgName
}

// RemoveDuplicates removes duplicate PURLs from a slice
func RemoveDuplicates(purls []LocalPurl) []LocalPurl {
	seen := make(map[string]bool)
	result := []LocalPurl{}

	for _, purl := range purls {
		key := purl.Purl
		if !seen[key] {
			seen[key] = true
			result = append(result, purl)
		}
	}

	return result
}

// BuildPURL constructs a Package URL string from components
func BuildPURL(purlType, namespace, name, version string, qualifiers map[string]string) string {
	var purl strings.Builder

	purl.WriteString("pkg:")
	purl.WriteString(purlType)
	purl.WriteString("/")

	if namespace != "" {
		purl.WriteString(namespace)
		purl.WriteString("/")
	}

	purl.WriteString(name)

	if version != "" {
		purl.WriteString("@")
		purl.WriteString(version)
	}

	if len(qualifiers) > 0 {
		purl.WriteString("?")
		first := true
		for k, v := range qualifiers {
			if !first {
				purl.WriteString("&")
			}
			purl.WriteString(k)
			purl.WriteString("=")
			purl.WriteString(v)
			first = false
		}
	}

	return purl.String()
}

// offsetToLine returns the 1-based line number of the byte at index off in content.
// It counts '\n' bytes strictly before off. off <= 0 clamps to line 1;
// off > len(content) clamps to the last line. Callers pass a real token offset
// so the common path is exact.
func offsetToLine(content []byte, off int) int {
	if off < 0 {
		off = 0
	}
	if off > len(content) {
		off = len(content)
	}
	return 1 + bytes.Count(content[:off], []byte{'\n'})
}

// lineTextAt returns the trimmed text of the physical line that contains byte
// index off in content. It walks backward to the preceding '\n' (or start) and
// forward to the next '\n' (or end), then trims whitespace.
func lineTextAt(content []byte, off int) string {
	if len(content) == 0 {
		return ""
	}
	if off < 0 {
		off = 0
	}
	if off >= len(content) {
		off = len(content) - 1
	}
	// Walk backward to find the start of this line
	start := off
	for start > 0 && content[start-1] != '\n' {
		start--
	}
	// Walk forward to find the end of this line
	end := off
	for end < len(content) && content[end] != '\n' {
		end++
	}
	return strings.TrimSpace(string(content[start:end]))
}

// GetFileType determines whether a file is a manifest or lockfile based on its name
func GetFileType(filename string) FileType {
	// Lockfiles (resolved/installed dependencies with exact versions)
	lockfiles := []string{
		"package-lock.json",
		"yarn.lock",
		"Gemfile.lock",
		"go.sum",
		// Note: Poetry's poetry.lock would go here but pyproject.toml is a manifest
	}

	for _, lockfile := range lockfiles {
		if filename == lockfile {
			return FileTypeLockfile
		}
	}

	// All other supported files are manifests (declared dependencies with version ranges)
	// package.json, go.mod, requirements.txt, pyproject.toml, pom.xml,
	// build.gradle, Gemfile, *.csproj, packages.config
	return FileTypeManifest
}
