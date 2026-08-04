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
	"encoding/xml"
	"path/filepath"
	"strings"
)

// csprojPackageReference represents a PackageReference element
type csprojPackageReference struct {
	Include string `xml:"Include,attr"`
	Version string `xml:"Version,attr"`
}

// packagesConfigPackage represents a package element in packages.config. There is no
// root type: the parser walks tokens to record each element's line rather than
// unmarshalling the whole document.
type packagesConfigPackage struct {
	ID      string `xml:"id,attr"`
	Version string `xml:"version,attr"`
}

// ParseCsproj parses a .csproj file and extracts NuGet package references using
// a streaming xml.Decoder token loop to capture the source line of each element.
func ParseCsproj(fileContent []byte, filePath string) (*LocalDependency, error) {
	result := &LocalDependency{
		File:     filePath,
		FileType: GetFileType(filepath.Base(filePath)),
		Purls:    []LocalPurl{},
	}

	d := xml.NewDecoder(bytes.NewReader(fileContent))

	for {
		// Capture the offset BEFORE reading the next token — this gives us the
		// start offset of the '<' of the element tag.
		prevOff := int(d.InputOffset())
		tok, err := d.Token()
		if err != nil {
			break
		}

		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		if start.Name.Local != "PackageReference" {
			continue
		}

		var elem csprojPackageReference
		if err := d.DecodeElement(&elem, &start); err != nil {
			return nil, err
		}
		endOff := int(d.InputOffset())

		if elem.Include == "" {
			continue
		}

		purl := BuildPURL("nuget", "", elem.Include, elem.Version, nil)
		result.Purls = append(result.Purls, LocalPurl{
			Purl:         purl,
			Requirement:  elem.Version,
			Line:         offsetToLine(fileContent, prevOff),
			DeclaredText: strings.TrimSpace(string(fileContent[prevOff:endOff])),
		})
	}

	return result, nil
}

// ParsePackagesConfig parses a packages.config file and extracts NuGet packages using
// a streaming xml.Decoder token loop to capture the source line of each element.
func ParsePackagesConfig(fileContent []byte, filePath string) (*LocalDependency, error) {
	result := &LocalDependency{
		File:     filePath,
		FileType: GetFileType(filepath.Base(filePath)),
		Purls:    []LocalPurl{},
	}

	d := xml.NewDecoder(bytes.NewReader(fileContent))

	for {
		prevOff := int(d.InputOffset())
		tok, err := d.Token()
		if err != nil {
			break
		}

		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		if start.Name.Local != "package" {
			continue
		}

		var elem packagesConfigPackage
		if err := d.DecodeElement(&elem, &start); err != nil {
			return nil, err
		}
		endOff := int(d.InputOffset())

		if elem.ID == "" {
			continue
		}

		purl := BuildPURL("nuget", "", elem.ID, elem.Version, nil)
		result.Purls = append(result.Purls, LocalPurl{
			Purl:         purl,
			Requirement:  elem.Version,
			Line:         offsetToLine(fileContent, prevOff),
			DeclaredText: strings.TrimSpace(string(fileContent[prevOff:endOff])),
		})
	}

	return result, nil
}

// ParseNuGet is the main entry point for parsing NuGet dependency files
func ParseNuGet(fileContent []byte, filePath string) (*LocalDependency, error) {
	filename := filepath.Base(filePath)

	if filename == "packages.config" {
		return ParsePackagesConfig(fileContent, filePath)
	} else if strings.HasSuffix(filename, ".csproj") {
		return ParseCsproj(fileContent, filePath)
	}

	// Return empty result for unsupported files
	return &LocalDependency{
		File:  filePath,
		Purls: []LocalPurl{},
	}, nil
}
