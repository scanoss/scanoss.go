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

package dependencies

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scanoss/scanoss.go/pkg/dependencies/parsers"
	"github.com/scanoss/scanoss.go/pkg/manifests"
)

// DependencyParser orchestrates parsing of dependency files across different ecosystems
type DependencyParser struct {
	parserMap map[string]parsers.ParserFunc
}

// NewDependencyParser creates a new instance of DependencyParser with all supported parsers
func NewDependencyParser() *DependencyParser {
	return &DependencyParser{
		// Keys come from pkg/manifests (the single source of truth shared with
		// pkg/filter). TestManifestsMatchParsers guards against drift.
		parserMap: map[string]parsers.ParserFunc{
			// Python
			manifests.RequirementsTxt: parsers.ParsePython,
			manifests.PyProjectToml:   parsers.ParsePython,

			// Java (Maven)
			manifests.PomXML: parsers.ParseMaven,

			// Java (Gradle)
			manifests.BuildGradle: parsers.ParseGradle,

			// Node.js
			manifests.PackageJSON:     parsers.ParseNPM,
			manifests.PackageLockJSON: parsers.ParseNPM,
			manifests.YarnLock:        parsers.ParseNPM,

			// Ruby
			manifests.Gemfile:     parsers.ParseRuby,
			manifests.GemfileLock: parsers.ParseRuby,

			// Go
			manifests.GoMod: parsers.ParseGolang,
			manifests.GoSum: parsers.ParseGolang,

			// .NET (NuGet)
			manifests.CsprojGlob:     parsers.ParseNuGet,
			manifests.PackagesConfig: parsers.ParseNuGet,
		},
	}
}

// GetParserFunc returns the appropriate parser function for a given file
func (dp *DependencyParser) GetParserFunc(filePath string) (parsers.ParserFunc, bool) {
	filename := filepath.Base(filePath)

	// Check for exact match first
	if parser, ok := dp.parserMap[filename]; ok {
		return parser, true
	}

	// Check for wildcard patterns
	for pattern, parser := range dp.parserMap {
		if strings.Contains(pattern, "*") {
			// Simple wildcard matching
			if matched, _ := filepath.Match(pattern, filename); matched {
				return parser, true
			}
		}
	}

	return nil, false
}

// FilterFiles returns only files that have a registered parser
func (dp *DependencyParser) FilterFiles(files []string) []string {
	filtered := []string{}

	for _, file := range files {
		if _, ok := dp.GetParserFunc(file); ok {
			filtered = append(filtered, file)
		}
	}

	return filtered
}

// ParseFile parses a single dependency file and returns the extracted dependencies
func (dp *DependencyParser) ParseFile(filePath string) (*parsers.LocalDependency, error) {
	// Get the appropriate parser
	parser, ok := dp.GetParserFunc(filePath)
	if !ok {
		return &parsers.LocalDependency{
			File:     filePath,
			FileType: parsers.GetFileType(filepath.Base(filePath)),
			Purls:    []parsers.LocalPurl{},
		}, nil
	}

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	// Parse the file
	result, err := parser(content, filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file %s: %w", filePath, err)
	}

	return result, nil
}

// ParseFiles parses multiple dependency files and returns aggregated results
func (dp *DependencyParser) ParseFiles(files []string) (*parsers.LocalDependencies, error) {
	result := &parsers.LocalDependencies{
		Files: []parsers.LocalDependency{},
	}

	for _, filePath := range files {
		dep, err := dp.ParseFile(filePath)
		if err != nil {
			// Log error but continue with other files
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			continue
		}

		// Only add if there are dependencies
		if len(dep.Purls) > 0 {
			result.Files = append(result.Files, *dep)
		}
	}

	return result, nil
}

// SupportedFiles returns a list of all supported dependency file patterns
func (dp *DependencyParser) SupportedFiles() []string {
	files := make([]string, 0, len(dp.parserMap))
	for pattern := range dp.parserMap {
		files = append(files, pattern)
	}
	return files
}

// IsSupportedFile checks if a file is supported by any parser
func (dp *DependencyParser) IsSupportedFile(filePath string) bool {
	_, ok := dp.GetParserFunc(filePath)
	return ok
}
