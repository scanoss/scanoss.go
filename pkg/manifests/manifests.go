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

// Package manifests is the single source of truth for the dependency-manifest
// file names/patterns the SDK understands (package.json, go.mod, pom.xml, …).
//
// It is a leaf package: it imports nothing internal, so both pkg/dependencies
// (which parses these files) and pkg/filter (which can preserve them while
// pruning everything else) can depend on it without a cycle. The dependency
// parser's registered files are kept in sync with Patterns by a drift-guard
// test (see pkg/dependencies).
package manifests

import (
	"path/filepath"
	"strings"
)

// Manifest file identifiers. Exact base names, plus glob patterns matched with
// filepath.Match (currently only "*.csproj"). Keep this list in sync with the
// dependency parser registrations in pkg/dependencies.
const (
	RequirementsTxt = "requirements.txt"
	PyProjectToml   = "pyproject.toml"
	PomXML          = "pom.xml"
	BuildGradle     = "build.gradle"
	PackageJSON     = "package.json"
	PackageLockJSON = "package-lock.json"
	YarnLock        = "yarn.lock"
	Gemfile         = "Gemfile"
	GemfileLock     = "Gemfile.lock"
	GoMod           = "go.mod"
	GoSum           = "go.sum"
	CsprojGlob      = "*.csproj"
	PackagesConfig  = "packages.config"
)

// Patterns is every manifest identifier (exact names and glob patterns).
var Patterns = []string{
	RequirementsTxt, PyProjectToml,
	PomXML, BuildGradle,
	PackageJSON, PackageLockJSON, YarnLock,
	Gemfile, GemfileLock,
	GoMod, GoSum,
	CsprojGlob, PackagesConfig,
}

// Is reports whether path's base name is a known dependency manifest. Matching
// is on the base name only, so nested manifests (e.g. "sub/pkg/package.json")
// are recognised.
func Is(path string) bool {
	base := filepath.Base(path)
	for _, p := range Patterns {
		if strings.ContainsRune(p, '*') {
			if ok, _ := filepath.Match(p, base); ok {
				return true
			}
			continue
		}
		if p == base {
			return true
		}
	}
	return false
}
