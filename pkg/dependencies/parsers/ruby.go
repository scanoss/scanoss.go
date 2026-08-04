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
	// gemfileGemRegex matches gem declarations in Gemfile
	// Example: gem 'rails' or gem "rails"
	gemfileGemRegex = regexp.MustCompile(`^\s*gem\s+["'](\w+)["']`)

	// gemfileLockSpecRegex matches gem specs in Gemfile.lock
	// Example: rails (7.0.0) or rails (7.0.0-x86_64-linux)
	gemfileLockSpecRegex = regexp.MustCompile(`^\s+(\w[\w\-]*)\s+\(([^\)]+)\)`)
)

// parserState represents the state machine for Gemfile.lock parsing
type parserState int

const (
	StateNone parserState = iota
	StateGEM
	StatePATH
	StateGIT
	StateSVN
	StateDEPENDENCIES
	StatePLATFORMS
)

// ParseGemfile parses a Gemfile and extracts gem dependencies
func ParseGemfile(fileContent []byte, filePath string) (*LocalDependency, error) {
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

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Match gem declaration
		if matches := gemfileGemRegex.FindStringSubmatch(line); len(matches) >= 2 {
			gemName := matches[1]

			purl := BuildPURL("gem", "", gemName, "", nil)

			result.Purls = append(result.Purls, LocalPurl{
				Purl:         purl,
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

// ParseGemfileLock parses a Gemfile.lock and extracts gem dependencies with versions
func ParseGemfileLock(fileContent []byte, filePath string) (*LocalDependency, error) {
	result := &LocalDependency{
		File:     filePath,
		FileType: GetFileType(filepath.Base(filePath)),
		Purls:    []LocalPurl{},
	}

	scanner := bufio.NewScanner(bytes.NewReader(fileContent))
	state := StateNone
	lineNo := 0

	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		trimmedLine := strings.TrimSpace(raw)

		// Skip empty lines and comments
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "#") {
			continue
		}

		// State transitions
		if strings.HasPrefix(raw, "GEM") {
			state = StateGEM
			continue
		} else if strings.HasPrefix(raw, "PATH") {
			state = StatePATH
			continue
		} else if strings.HasPrefix(raw, "GIT") {
			state = StateGIT
			continue
		} else if strings.HasPrefix(raw, "SVN") {
			state = StateSVN
			continue
		} else if strings.HasPrefix(raw, "DEPENDENCIES") {
			state = StateDEPENDENCIES
			continue
		} else if strings.HasPrefix(raw, "PLATFORMS") {
			state = StatePLATFORMS
			continue
		}

		// Only process specs in GEM section
		if state == StateGEM {
			// Look for "specs:" marker
			if strings.Contains(trimmedLine, "specs:") {
				continue
			}

			// Match gem spec lines
			if matches := gemfileLockSpecRegex.FindStringSubmatch(raw); len(matches) >= 3 {
				gemName := matches[1]
				versionSpec := matches[2]

				// Extract version and platform if present
				// Example: "7.0.0-x86_64-linux" -> version: "7.0.0", platform: "x86_64-linux"
				parts := strings.Split(versionSpec, "-")
				version := parts[0]

				purl := BuildPURL("gem", "", gemName, version, nil)

				result.Purls = append(result.Purls, LocalPurl{
					Purl:         purl,
					Requirement:  version,
					Line:         lineNo,
					DeclaredText: strings.TrimSpace(raw),
				})
			}
		}

		// Skip PATH dependencies (local gems)
		if state == StatePATH {
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// ParseRuby is the main entry point for parsing Ruby dependency files
func ParseRuby(fileContent []byte, filePath string) (*LocalDependency, error) {
	filename := filepath.Base(filePath)

	switch filename {
	case "Gemfile":
		return ParseGemfile(fileContent, filePath)
	case "Gemfile.lock":
		return ParseGemfileLock(fileContent, filePath)
	default:
		// Return empty result for unsupported files
		return &LocalDependency{
			File:  filePath,
			Purls: []LocalPurl{},
		}, nil
	}
}
