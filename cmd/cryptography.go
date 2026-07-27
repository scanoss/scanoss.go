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

func callCryptoAlgorithms(c *scanoss.Client, ctx context.Context, comps []scanoss.Component) (*scanossapi.CryptoAlgorithmsResponse, error) {
	return c.Cryptography.Algorithms(ctx, comps)
}

func callCryptoAlgorithmsRange(c *scanoss.Client, ctx context.Context, comps []scanoss.Component) (*scanossapi.CryptoAlgorithmsInRangeResponse, error) {
	return c.Cryptography.AlgorithmsInRange(ctx, comps)
}

func callCryptoVersionsRange(c *scanoss.Client, ctx context.Context, comps []scanoss.Component) (*scanossapi.CryptoVersionsInRangeResponse, error) {
	return c.Cryptography.VersionsInRange(ctx, comps)
}

func callCryptoHints(c *scanoss.Client, ctx context.Context, comps []scanoss.Component) (*scanossapi.CryptoHintsResponse, error) {
	return c.Cryptography.Hints(ctx, comps)
}

func callCryptoHintsRange(c *scanoss.Client, ctx context.Context, comps []scanoss.Component) (*scanossapi.CryptoHintsInRangeResponse, error) {
	return c.Cryptography.HintsInRange(ctx, comps)
}

var cryptographyCmd = &cobra.Command{
	Use:   "cryptography",
	Short: "Query cryptographic algorithms and libraries from the SCANOSS API",
	Long: `The cryptography command queries the SCANOSS API for cryptographic
algorithms and libraries (hints) used in software components.

Operations are subcommands:
  algorithms        algorithms at the resolved version (default)
  algorithms-range  algorithms across a version range
  versions-range    versions in a range, split by crypto presence
  hints             crypto library hints at the resolved version
  hints-range       crypto library hints across a version range

The version (or range expression) is provided via --requirement (or per-purl in
--input). The input is a list of PURLs, queried in chunks by a pool of workers.

Examples:
  # Algorithms at a version (default)
  scanoss-cli cryptography --purl 'pkg:github/scanoss/engine' --requirement '5.0.1'

  # Crypto library hints across a range
  scanoss-cli cryptography hints-range --purl 'pkg:github/scanoss/engine' --requirement '>5.0.0'`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error { return runPurlServiceTyped(cmd, callCryptoAlgorithms) },
}

func init() {
	rootCmd.AddCommand(cryptographyCmd)
	addPurlServiceFlags(cryptographyCmd)
	cryptographyCmd.AddCommand(
		newPurlServiceCmdTyped("algorithms", "Cryptographic algorithms (default)",
			"Query cryptographic algorithms detected at the resolved version.", callCryptoAlgorithms),
		newPurlServiceCmdTyped("algorithms-range", "Algorithms across a version range",
			"Query cryptographic algorithms across every version satisfying the range.", callCryptoAlgorithmsRange),
		newPurlServiceCmdTyped("versions-range", "Versions in a range split by crypto presence",
			"Partition the versions in a range into those with and without crypto.", callCryptoVersionsRange),
		newPurlServiceCmdTyped("hints", "Cryptographic library hints",
			"Query cryptographic library hints detected at the resolved version.", callCryptoHints),
		newPurlServiceCmdTyped("hints-range", "Library hints across a version range",
			"Query cryptographic library hints across every version satisfying the range.", callCryptoHintsRange),
	)
}
