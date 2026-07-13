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
	"bufio"
	"bytes"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// requirementRegex matches Python requirement lines
	// Example: django>=4.2.0 or requests==2.28.0 or flask
	requirementRegex = regexp.MustCompile(`^(?P<name>[-\w]+)\s*(?P<sym>[>=~!]*)\s*(?P<version>[\d\.]*)`)

	// pyprojectDepsRegex matches dependency lines in pyproject.toml
	// Example: "django>=4.2.0" or 'requests==2.28.0'
	pyprojectDepsRegex = regexp.MustCompile(`["']([^"']+)["']`)
)

// ParseRequirementsTxt parses a requirements.txt file and extracts dependencies
func ParseRequirementsTxt(fileContent []byte, filePath string) (*LocalDependency, error) {
	result := &LocalDependency{
		File:     filePath,
		FileType: GetFileType(filepath.Base(filePath)),
		Purls:    []LocalPurl{},
	}

	scanner := bufio.NewScanner(bytes.NewReader(fileContent))
	lineNo := 0

	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		line := strings.TrimSpace(raw)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Skip comments
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Skip URLs
		if IsValidURL(line) {
			continue
		}

		// Skip local file paths
		if IsValidPath(line) {
			continue
		}

		// Skip recursive dependencies (-r requirements-dev.txt)
		if strings.HasPrefix(line, "-r") || strings.HasPrefix(line, "--requirement") {
			continue
		}

		// Skip editable installs (-e .)
		if strings.HasPrefix(line, "-e") || strings.HasPrefix(line, "--editable") {
			continue
		}

		// Parse the requirement
		matches := requirementRegex.FindStringSubmatch(line)
		if len(matches) >= 4 {
			name := matches[1]
			symbol := matches[2]
			version := matches[3]

			var purl string
			var requirement string

			// Build PURL based on version specifier
			if symbol == "" {
				// No version specified
				purl = BuildPURL("pypi", "", name, "", nil)
			} else if symbol == "==" && version != "" {
				// Exact version
				purl = BuildPURL("pypi", "", name, version, nil)
				requirement = version
			} else if version != "" {
				// Other operators (>=, ~, !=, etc.)
				purl = BuildPURL("pypi", "", name, "", nil)
				requirement = symbol + version
			} else {
				// Just the symbol without version
				purl = BuildPURL("pypi", "", name, "", nil)
			}

			result.Purls = append(result.Purls, LocalPurl{
				Purl:         purl,
				Requirement:  requirement,
				Line:         lineNo,
				DeclaredText: strings.TrimSpace(raw),
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// ParsePyprojectToml parses a pyproject.toml file and extracts dependencies
func ParsePyprojectToml(fileContent []byte, filePath string) (*LocalDependency, error) {
	result := &LocalDependency{
		File:     filePath,
		FileType: GetFileType(filepath.Base(filePath)),
		Purls:    []LocalPurl{},
	}

	scanner := bufio.NewScanner(bytes.NewReader(fileContent))
	inDependenciesSection := false
	lineNo := 0

	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		line := strings.TrimSpace(raw)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for dependencies section
		if strings.Contains(line, "dependencies") && strings.Contains(line, "[") {
			inDependenciesSection = true
			continue
		}

		// Check for end of section (next section starts with [)
		if inDependenciesSection && strings.HasPrefix(line, "[") {
			inDependenciesSection = false
			continue
		}

		// Parse dependencies within the section
		if inDependenciesSection {
			// Extract quoted strings; all deps on this physical line share lineNo and raw.
			matches := pyprojectDepsRegex.FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				if len(match) >= 2 {
					dep := match[1]

					// Parse the dependency (similar to requirements.txt)
					reqMatches := requirementRegex.FindStringSubmatch(dep)
					if len(reqMatches) >= 4 {
						name := reqMatches[1]
						symbol := reqMatches[2]
						version := reqMatches[3]

						var purl string
						var requirement string

						if symbol == "" {
							purl = BuildPURL("pypi", "", name, "", nil)
						} else if symbol == "==" && version != "" {
							purl = BuildPURL("pypi", "", name, version, nil)
							requirement = version
						} else if version != "" {
							purl = BuildPURL("pypi", "", name, "", nil)
							requirement = symbol + version
						} else {
							purl = BuildPURL("pypi", "", name, "", nil)
						}

						result.Purls = append(result.Purls, LocalPurl{
							Purl:         purl,
							Requirement:  requirement,
							Line:         lineNo,
							DeclaredText: strings.TrimSpace(raw),
						})
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// ParsePython is the main entry point for parsing Python dependency files
func ParsePython(fileContent []byte, filePath string) (*LocalDependency, error) {
	filename := filepath.Base(filePath)

	switch filename {
	case "requirements.txt":
		return ParseRequirementsTxt(fileContent, filePath)
	case "pyproject.toml":
		return ParsePyprojectToml(fileContent, filePath)
	default:
		// Return empty result for unsupported files
		return &LocalDependency{
			File:  filePath,
			Purls: []LocalPurl{},
		}, nil
	}
}
