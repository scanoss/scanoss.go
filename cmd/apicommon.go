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

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/scanoss/scanoss.go/pkg/scanoss"
)

// addAPIFlags registers the flags every command that talks to the SCANOSS API
// needs. It is the single declaration of these four: a new one is added here and
// reaches every such command, instead of being remembered in a dozen files.
//
// They are persistent so a parent command's subcommands inherit them — which is
// what gives `scan wfp` and the `components` subcommands their API flags. On a
// command with no subcommands persistent and local flags behave the same, so
// every caller can use this regardless of its shape.
func addAPIFlags(cmd *cobra.Command) {
	fs := cmd.PersistentFlags()
	fs.String("api-url", scanoss.DefaultAPIURL, "SCANOSS API base URL")
	fs.String("api-key", "", "API key for authentication")
	fs.Bool("ignore-cert-errors", false, "Ignore TLS certificate errors (insecure)")
	fs.StringP("output", "o", "", "Output file (default: stdout)")
}
