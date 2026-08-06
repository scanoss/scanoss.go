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
	"context"

	scanossapi "github.com/scanoss/scanoss.api-sdk"
	"github.com/spf13/cobra"

	"github.com/scanoss/scanoss.go/pkg/scanoss"
)

func callCopyrightEvidence(c *scanoss.Client, ctx context.Context, comps []scanoss.Component, opts ...scanoss.DecorateOption) (*scanossapi.CopyrightEvidenceResponse, error) {
	return c.Copyright.Evidence(ctx, comps, opts...)
}

func callCopyrightHolders(c *scanoss.Client, ctx context.Context, comps []scanoss.Component, opts ...scanoss.DecorateOption) (*scanossapi.CopyrightHoldersResponse, error) {
	return c.Copyright.Holders(ctx, comps, opts...)
}

var copyrightCmd = &cobra.Command{
	Use:   "copyright",
	Short: "Query copyright information for components from the SCANOSS API",
	Long: `The copyright command queries the SCANOSS API for copyright
information about one or more components.

Operations are subcommands:
  evidence  per-file copyright evidence (default)
  holders   distinct copyright holders

The input is a list of PURLs, split into chunks and sent concurrently by a pool
of workers.

Examples:
  # Per-file copyright evidence (default)
  scanoss-cli copyright --purl 'pkg:github/scanoss/engine' --requirement '5.4.7'

  # Distinct copyright holders
  scanoss-cli copyright holders --purl 'pkg:github/scanoss/engine' --requirement '5.4.7'`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error { return runPurlServiceTyped(cmd, callCopyrightEvidence) },
}

func init() {
	rootCmd.AddCommand(copyrightCmd)
	addPurlServiceFlags(copyrightCmd)
	copyrightCmd.AddCommand(
		newPurlServiceCmdTyped("evidence", "Per-file copyright evidence (default)",
			"Query per-file copyright evidence for the given components.", callCopyrightEvidence),
		newPurlServiceCmdTyped("holders", "Distinct copyright holders",
			"Query the distinct copyright holders for the given components.", callCopyrightHolders),
	)
}
