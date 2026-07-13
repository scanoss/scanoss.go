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
	"testing"
)

// TestParseCsprojSourceLocation verifies Line and DeclaredText for .csproj files.
func TestParseCsprojSourceLocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    []struct {
			purl         string
			requirement  string
			line         int
			declaredText string
		}
	}{
		{
			name: "self-closing PackageReference on a single line",
			// line 1: <Project Sdk="Microsoft.NET.Sdk">
			// line 2:   <ItemGroup>
			// line 3:     <PackageReference Include="Newtonsoft.Json" Version="13.0.1" />
			// line 4:   </ItemGroup>
			// line 5: </Project>
			content: `<Project Sdk="Microsoft.NET.Sdk">
  <ItemGroup>
    <PackageReference Include="Newtonsoft.Json" Version="13.0.1" />
  </ItemGroup>
</Project>
`,
			want: []struct {
				purl         string
				requirement  string
				line         int
				declaredText string
			}{
				{
					purl:         "pkg:nuget/Newtonsoft.Json@13.0.1",
					requirement:  "13.0.1",
					line:         3,
					declaredText: `<PackageReference Include="Newtonsoft.Json" Version="13.0.1" />`,
				},
			},
		},
		{
			name: "multiple PackageReference entries",
			// line 1: <Project>
			// line 2:   <ItemGroup>
			// line 3:     <PackageReference Include="Serilog" Version="3.0.0" />
			// line 4:     <PackageReference Include="FluentValidation" Version="11.0.0" />
			// line 5:   </ItemGroup>
			// line 6: </Project>
			content: `<Project>
  <ItemGroup>
    <PackageReference Include="Serilog" Version="3.0.0" />
    <PackageReference Include="FluentValidation" Version="11.0.0" />
  </ItemGroup>
</Project>
`,
			want: []struct {
				purl         string
				requirement  string
				line         int
				declaredText string
			}{
				{
					purl:         "pkg:nuget/Serilog@3.0.0",
					requirement:  "3.0.0",
					line:         3,
					declaredText: `<PackageReference Include="Serilog" Version="3.0.0" />`,
				},
				{
					purl:         "pkg:nuget/FluentValidation@11.0.0",
					requirement:  "11.0.0",
					line:         4,
					declaredText: `<PackageReference Include="FluentValidation" Version="11.0.0" />`,
				},
			},
		},
		{
			name: "zero-value case — no PackageReference elements",
			content: `<Project>
  <PropertyGroup>
    <TargetFramework>net8.0</TargetFramework>
  </PropertyGroup>
</Project>
`,
			want: []struct {
				purl         string
				requirement  string
				line         int
				declaredText string
			}{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := ParseCsproj([]byte(tc.content), "MyApp.csproj")
			if err != nil {
				t.Fatalf("ParseCsproj error: %v", err)
			}
			if len(result.Purls) != len(tc.want) {
				t.Fatalf("got %d purls, want %d; purls: %+v", len(result.Purls), len(tc.want), result.Purls)
			}
			for i, got := range result.Purls {
				w := tc.want[i]
				if got.Purl != w.purl {
					t.Errorf("[%d] Purl: got %q, want %q", i, got.Purl, w.purl)
				}
				if got.Requirement != w.requirement {
					t.Errorf("[%d] Requirement: got %q, want %q", i, got.Requirement, w.requirement)
				}
				if got.Line != w.line {
					t.Errorf("[%d] Line: got %d, want %d", i, got.Line, w.line)
				}
				if got.DeclaredText != w.declaredText {
					t.Errorf("[%d] DeclaredText: got %q, want %q", i, got.DeclaredText, w.declaredText)
				}
			}
		})
	}
}

// TestParsePackagesConfigZeroLine verifies that a packages.config with no <package> elements
// returns zero purls, mirroring the zero-value case in TestParseCsprojSourceLocation.
func TestParsePackagesConfigZeroLine(t *testing.T) {
	t.Parallel()

	// Only the XML header and root element — no <package> entries.
	content := `<?xml version="1.0" encoding="utf-8"?>
<packages>
</packages>
`
	result, err := ParsePackagesConfig([]byte(content), "packages.config")
	if err != nil {
		t.Fatalf("ParsePackagesConfig error: %v", err)
	}
	if len(result.Purls) != 0 {
		t.Errorf("expected 0 purls for packages.config with no entries, got %d: %+v", len(result.Purls), result.Purls)
	}
}

// TestParsePackagesConfigSourceLocation verifies Line and DeclaredText for packages.config.
func TestParsePackagesConfigSourceLocation(t *testing.T) {
	t.Parallel()

	// line 1: <?xml version="1.0" encoding="utf-8"?>
	// line 2: <packages>
	// line 3:   <package id="Newtonsoft.Json" version="13.0.1" targetFramework="net48" />
	// line 4:   <package id="Serilog" version="3.0.0" targetFramework="net48" />
	// line 5: </packages>
	content := `<?xml version="1.0" encoding="utf-8"?>
<packages>
  <package id="Newtonsoft.Json" version="13.0.1" targetFramework="net48" />
  <package id="Serilog" version="3.0.0" targetFramework="net48" />
</packages>
`
	result, err := ParsePackagesConfig([]byte(content), "packages.config")
	if err != nil {
		t.Fatalf("ParsePackagesConfig error: %v", err)
	}
	if len(result.Purls) != 2 {
		t.Fatalf("got %d purls, want 2; purls: %+v", len(result.Purls), result.Purls)
	}
	if result.Purls[0].Line != 3 {
		t.Errorf("first entry Line: got %d, want 3", result.Purls[0].Line)
	}
	if result.Purls[0].DeclaredText == "" {
		t.Errorf("first entry DeclaredText is empty")
	}
	if result.Purls[1].Line != 4 {
		t.Errorf("second entry Line: got %d, want 4", result.Purls[1].Line)
	}
	if result.Purls[1].DeclaredText == "" {
		t.Errorf("second entry DeclaredText is empty")
	}
}
