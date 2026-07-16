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

	"github.com/spf13/cobra"

	scanossapi "github.com/scanoss/scanoss.api-sdk"
	"github.com/scanoss/scanoss.go/pkg/scanoss"
)

func callLicenseDeclared(c *scanoss.Client, ctx context.Context, comps []scanoss.Component) (*scanossapi.ComponentsLicenseResponse, error) {
	return c.Licenses.Components(ctx, comps)
}

func callLicenseAttribution(c *scanoss.Client, ctx context.Context, comps []scanoss.Component) (*scanossapi.AttributionResponse, error) {
	return c.Licenses.Attribution(ctx, comps)
}

func callLicenseEvidence(c *scanoss.Client, ctx context.Context, comps []scanoss.Component) (*scanossapi.LicenseEvidenceResponse, error) {
	return c.Licenses.Evidence(ctx, comps)
}

var licensesCmd = &cobra.Command{
	Use:   "licenses",
	Short: "Query license information for components from the SCANOSS API",
	Long: `The licenses command queries the SCANOSS API for license information
about one or more components.

Operations are subcommands:
  declared     declared licenses for the components (default)
  attribution  attribution files: LICENSE/NOTICE/…
  evidence     per-file license evidence

The input is a list of PURLs, split into chunks and sent concurrently by a pool
of workers.

Examples:
  # Declared licenses (default)
  scanoss-cli licenses --purl 'pkg:github/scanoss/engine' --requirement '5.4.7'

  # Attribution files
  scanoss-cli licenses attribution --purl 'pkg:github/scanoss/engine'

  # Per-file license evidence
  scanoss-cli licenses evidence --purl 'pkg:github/scanoss/engine'`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error { return runPurlServiceTyped(cmd, callLicenseDeclared) },
}

func init() {
	rootCmd.AddCommand(licensesCmd)
	addPurlServiceFlags(licensesCmd)
	licensesCmd.AddCommand(
		newPurlServiceCmdTyped("declared", "Declared licenses for the components (default)",
			"Query the declared licenses for the given components.", callLicenseDeclared),
		newPurlServiceCmdTyped("attribution", "Attribution files (LICENSE/NOTICE/…)",
			"Query the attribution files (LICENSE/NOTICE/…) for the given components.", callLicenseAttribution),
		newPurlServiceCmdTyped("evidence", "Per-file license evidence",
			"Query per-file license evidence for the given components.", callLicenseEvidence),
	)
}
