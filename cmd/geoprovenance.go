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

func callGeoOrigin(c *scanoss.Client, ctx context.Context, comps []scanoss.Component, opts ...scanoss.DecorateOption) (*scanossapi.GeoOriginResponse, error) {
	return c.Geoprovenance.Origins(ctx, comps, opts...)
}

func callGeoCountries(c *scanoss.Client, ctx context.Context, comps []scanoss.Component, opts ...scanoss.DecorateOption) (*scanossapi.GeoContributorsResponse, error) {
	return c.Geoprovenance.Countries(ctx, comps, opts...)
}

var geoprovenanceCmd = &cobra.Command{
	Use:   "geoprovenance",
	Short: "Query geographic provenance for components from the SCANOSS API",
	Long: `The geoprovenance command queries the SCANOSS API for the geographic
provenance of one or more components.

Operations are subcommands:
  origin     country of origin of the component (default)
  countries  contributor countries of the component

The input is a list of PURLs, split into chunks and sent concurrently by a pool
of workers.

Examples:
  # Component origin (default)
  scanoss-cli geoprovenance --purl 'pkg:github/scanoss/engine' --requirement '5.4.7'

  # Contributor countries for several components
  scanoss-cli geoprovenance countries --purl 'pkg:github/scanoss/engine'`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error { return runPurlServiceTyped(cmd, callGeoOrigin) },
}

func init() {
	rootCmd.AddCommand(geoprovenanceCmd)
	addPurlServiceFlags(geoprovenanceCmd)
	geoprovenanceCmd.AddCommand(
		newPurlServiceCmdTyped("origin", "Component country of origin (default)",
			"Query the country of origin of the given components.", callGeoOrigin),
		newPurlServiceCmdTyped("countries", "Contributor countries",
			"Query the contributor countries of the given components.", callGeoCountries),
	)
}
