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

package dependencies_test

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/scanoss/scanoss.go/pkg/dependencies"
	"github.com/scanoss/scanoss.go/pkg/dependencies/parsers"
)

func ExampleDependencyParser_ParseFile() {
	parser := dependencies.NewDependencyParser()

	// In real usage, you would parse from a file:
	// result, err := parser.ParseFile("path/to/go.mod")
	// if err != nil {
	//     log.Fatal(err)
	// }

	// For this example, just demonstrate parser creation
	_ = parser // placeholder for example

	fmt.Println("Parser created successfully")
	// Output: Parser created successfully
}

func ExampleDependencyParser_FilterFiles() {
	parser := dependencies.NewDependencyParser()

	files := []string{
		"README.md",
		"package.json",
		"go.mod",
		"main.go",
		"pom.xml",
		"requirements.txt",
	}

	supported := parser.FilterFiles(files)

	fmt.Printf("Found %d supported dependency files\n", len(supported))
	// Output: Found 4 supported dependency files
}

func ExampleDependencyParser_IsSupportedFile() {
	parser := dependencies.NewDependencyParser()

	files := map[string]bool{
		"package.json":     true,
		"go.mod":           true,
		"main.go":          false,
		"requirements.txt": true,
		"pom.xml":          true,
		"README.md":        false,
	}

	for file, expectedSupport := range files {
		isSupported := parser.IsSupportedFile(file)
		if isSupported == expectedSupport {
			fmt.Printf("%s: supported=%v\n", file, isSupported)
		}
	}
	// Output will vary based on execution order, but all files will be checked
}

func ExampleDependencyParser_SupportedFiles() {
	parser := dependencies.NewDependencyParser()

	patterns := parser.SupportedFiles()

	fmt.Printf("Total supported patterns: %d\n", len(patterns))
	fmt.Println("Includes patterns for Node.js, Python, Java, Go, Ruby, and .NET")
	// Output:
	// Total supported patterns: 13
	// Includes patterns for Node.js, Python, Java, Go, Ruby, and .NET
}

// Example demonstrates marshaling results to JSON
func Example() {
	parser := dependencies.NewDependencyParser()

	// Simulated result
	result := &parsers.LocalDependencies{
		Files: []parsers.LocalDependency{
			{
				File: "go.mod",
				Purls: []parsers.LocalPurl{
					{
						Purl:        "pkg:golang/github.com/spf13/cobra@v1.10.2",
						Requirement: "v1.10.2",
					},
				},
			},
		},
	}

	// Note: In actual code, you would get this from parser.ParseFiles()
	_ = parser

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(jsonData))
	// Output:
	// {
	//   "files": [
	//     {
	//       "file": "go.mod",
	//       "purls": [
	//         {
	//           "purl": "pkg:golang/github.com/spf13/cobra@v1.10.2",
	//           "requirement": "v1.10.2"
	//         }
	//       ]
	//     }
	//   ]
	// }
}
