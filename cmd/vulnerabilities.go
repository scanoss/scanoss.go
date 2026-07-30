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

func callVulnComponents(c *scanoss.Client, ctx context.Context, comps []scanoss.Component, opts ...scanoss.DecorateOption) (*scanossapi.VulnerabilitiesResponse, error) {
	return c.Vulnerabilities.Components(ctx, comps, opts...)
}

func callVulnCpes(c *scanoss.Client, ctx context.Context, comps []scanoss.Component, opts ...scanoss.DecorateOption) (*scanossapi.CpesResponse, error) {
	return c.Vulnerabilities.Cpes(ctx, comps, opts...)
}

var vulnerabilitiesCmd = &cobra.Command{
	Use:   "vulnerabilities",
	Short: "Query vulnerabilities for components from the SCANOSS API",
	Long: `The vulnerabilities command queries the SCANOSS API for known
vulnerabilities affecting one or more components.

Operations are subcommands:
  components  known vulnerabilities (default)
  cpes        CPEs detected for the components

Run without a subcommand to use the default (components). The input is a list of
PURLs, split into chunks and sent concurrently by a pool of workers.

Examples:
  # Vulnerabilities (default)
  scanoss-cli vulnerabilities --purl 'pkg:github/scanoss/engine' --requirement '5.4.7'

  # CPEs
  scanoss-cli vulnerabilities cpes --purl 'pkg:github/scanoss/engine'

  # From a file, in chunks of 20 with up to 10 workers
  scanoss-cli vulnerabilities --input purls.txt --chunk-size 20 --workers 10`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error { return runPurlServiceTyped(cmd, callVulnComponents) },
}

func init() {
	rootCmd.AddCommand(vulnerabilitiesCmd)
	addPurlServiceFlags(vulnerabilitiesCmd)
	vulnerabilitiesCmd.AddCommand(
		newPurlServiceCmdTyped("components", "Known vulnerabilities for components (default)",
			"Query known vulnerabilities affecting the given components.", callVulnComponents),
		newPurlServiceCmdTyped("cpes", "CPEs detected for components",
			"Query the CPEs detected for the given components.", callVulnCpes),
	)
}
