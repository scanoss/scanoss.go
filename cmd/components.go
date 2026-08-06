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
	"encoding/json"
	"fmt"

	scanossapi "github.com/scanoss/scanoss.api-sdk"
	"github.com/spf13/cobra"

	"github.com/scanoss/scanoss.go/pkg/scanoss"
)

func callComponentsStatus(c *scanoss.Client, ctx context.Context, comps []scanoss.Component, opts ...scanoss.DecorateOption) (*scanossapi.ComponentsStatusResponse, error) {
	return c.Components.Status(ctx, comps, opts...)
}

// writeTyped marshals a typed response model to indented JSON and writes it to
// the --output file (or stdout). Used by the single-shot components commands.
func writeTyped(cmd *cobra.Command, v any) error {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	outputFile, _ := cmd.Flags().GetString("output")
	return writeOutput(string(out), outputFile)
}

// addSearchFlags registers the flags for `components search` (and the parent's
// default search). Local flags — the operation does not fan out over a PURL list.
func addSearchFlags(cmd *cobra.Command) {
	fs := cmd.Flags()
	fs.String("search", "", "Free-form search term (overrides --vendor/--component)")
	fs.String("vendor", "", "Filter by vendor")
	fs.String("component", "", "Filter by component name")
	fs.String("purl-type", "", "purl type (github, npm, pypi, …); defaults to github server-side")
	fs.Int("limit", 0, "Maximum number of results (0 = server default)")
	fs.Int("offset", 0, "Pagination offset")
}

func runComponentsSearch(cmd *cobra.Command, _ []string) error {
	search, _ := cmd.Flags().GetString("search")
	vendor, _ := cmd.Flags().GetString("vendor")
	component, _ := cmd.Flags().GetString("component")
	if search == "" && vendor == "" && component == "" {
		// Show usage, not the auth banner: the key is not what is missing.
		return usageError(cmd, "--search, --vendor or --component is required")
	}
	if err := checkAuth(cmd); err != nil {
		return err
	}
	purlType, _ := cmd.Flags().GetString("purl-type")
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")

	client, err := newClient(cmd)
	if err != nil {
		return err
	}
	res, err := client.Components.Search(cmd.Context(), scanoss.ComponentSearch{
		Search:    search,
		Vendor:    vendor,
		Component: component,
		PurlType:  purlType,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return renderAPIError(err)
	}
	return writeTyped(cmd, res)
}

func runComponentsVersions(cmd *cobra.Command, _ []string) error {
	purl, _ := cmd.Flags().GetString("purl")
	if purl == "" {
		// Show usage, not the auth banner: the key is not what is missing.
		return usageError(cmd, "--purl is required")
	}
	if err := checkAuth(cmd); err != nil {
		return err
	}
	limit, _ := cmd.Flags().GetInt("limit")

	client, err := newClient(cmd)
	if err != nil {
		return err
	}
	res, err := client.Components.Versions(cmd.Context(), purl, limit)
	if err != nil {
		return renderAPIError(err)
	}
	return writeTyped(cmd, res)
}

func runComponentsReleases(cmd *cobra.Command, _ []string) error {
	purl, _ := cmd.Flags().GetString("purl")
	if purl == "" {
		// Show usage, not the auth banner: the key is not what is missing.
		return usageError(cmd, "--purl is required")
	}
	if err := checkAuth(cmd); err != nil {
		return err
	}
	requirement, _ := cmd.Flags().GetString("requirement")
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")

	client, err := newClient(cmd)
	if err != nil {
		return err
	}
	res, err := client.Components.Releases(cmd.Context(), purl, requirement, limit, offset)
	if err != nil {
		return renderAPIError(err)
	}
	// RELEASE_NOTES_UNAVAILABLE means the component exists but has no notes for the
	// resolved version — an explicit outcome, not an error. Note it on stderr and
	// still emit the (machine-readable) JSON on stdout with a zero exit.
	if ic := res.Component.InfoCode; ic != nil && *ic == scanossapi.InfoCodeRELEASENOTESUNAVAILABLE {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), releaseNotesUnavailableMsg(res.Component))
	}
	return writeTyped(cmd, res)
}

// releaseNotesUnavailableMsg builds the "no release notes available" notice,
// naming the resolved version when the API echoed one.
func releaseNotesUnavailableMsg(c scanossapi.ReleaseComponent) string {
	target := ""
	if c.Purl != nil {
		target = *c.Purl
	}
	if c.Version != nil && *c.Version != "" {
		target += "@" + *c.Version
	}
	if target == "" {
		return "no release notes available"
	}
	return "no release notes available for " + target
}

var componentsCmd = &cobra.Command{
	Use:   "components",
	Short: "Search components and query versions/status from the SCANOSS API",
	Long: `The components command queries the SCANOSS API for component metadata.

Operations are subcommands:
  search    find components by term/vendor/component (default)
  versions  list known versions (with licenses) for a purl
  releases  list release / changelog entries for a purl
  status    resolve the lifecycle status for components

Examples:
  # Search (default)
  scanoss-cli components --vendor scanoss --component engine --limit 20

  # Versions for a purl
  scanoss-cli components versions --purl 'pkg:github/scanoss/engine' --limit 50

  # Release notes for a purl (all, a single version, or a range)
  scanoss-cli components releases --purl 'pkg:github/scanoss/engine'
  scanoss-cli components releases --purl 'pkg:github/scanoss/engine' --requirement '5.4.7'
  scanoss-cli components releases --purl 'pkg:github/scanoss/engine' --requirement '>=1.0.0, <=2.0.0' --limit 10

  # Lifecycle status for components
  scanoss-cli components status --purl 'pkg:github/scanoss/engine' --requirement '5.4.7'`,
	Args: cobra.NoArgs,
	RunE: runComponentsSearch,
}

func init() {
	rootCmd.AddCommand(componentsCmd)
	addAPIFlags(componentsCmd) // persistent: inherited by every subcommand
	addSearchFlags(componentsCmd)

	searchCmd := &cobra.Command{
		Use:   "search",
		Short: "Search components (default)",
		Long:  "Search components by free term, vendor and/or component name.",
		Args:  cobra.NoArgs,
		RunE:  runComponentsSearch,
	}
	addSearchFlags(searchCmd)

	versionsCmd := &cobra.Command{
		Use:   "versions",
		Short: "List versions for a purl",
		Long:  "List the known versions (with licenses) for a purl, most recent first.",
		Args:  cobra.NoArgs,
		RunE:  runComponentsVersions,
	}
	versionsCmd.Flags().String("purl", "", "Package URL (purl) to list versions for")
	versionsCmd.Flags().Int("limit", 0, "Maximum number of versions (0 = server default)")

	releasesCmd := &cobra.Command{
		Use:   "releases",
		Short: "List release notes for a purl",
		Long: `List the release / changelog entries (version, date, notes, url) for a purl.

Without --requirement, all releases are listed. Pass --requirement with an exact
version (e.g. '5.4.7') to resolve a single version, or a semver range (e.g.
'>=1.0.0, <=2.0.0') to narrow to a range. --limit/--offset paginate the results.`,
		Args: cobra.NoArgs,
		RunE: runComponentsReleases,
	}
	releasesCmd.Flags().String("purl", "", "Package URL (purl) to list releases for")
	releasesCmd.Flags().String("requirement", "", "Version requirement: an exact version or a semver range (e.g. '>=1.0.0, <=2.0.0')")
	releasesCmd.Flags().Int("limit", 0, "Maximum number of releases (0 = server default)")
	releasesCmd.Flags().Int("offset", 0, "Pagination offset")

	statusCmd := newPurlServiceCmdTyped("status", "Lifecycle status for components",
		"Resolve the lifecycle status for the given components.", callComponentsStatus)
	addPurlInputFlags(statusCmd)

	componentsCmd.AddCommand(searchCmd, versionsCmd, releasesCmd, statusCmd)
}
