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
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/scanoss/scanoss.go/pkg/scanoss"
)

// errNoAPIKey is returned by the no-key guard after it has already printed the
// banner. Execute recognizes it and exits non-zero without printing anything more.
var errNoAPIKey = errors.New("no API key provided for the default SCANOSS endpoint")

// scanossNoKeyBanner is shown when a user targets the default SCANOSS endpoint
// without an API key.
const scanossNoKeyBanner = `
   ____   ____    _    _   _  ___  ____ ____
  / ___| / ___|  / \  | \ | |/ _ \/ ___/ ___|
  \___ \| |     / _ \ |  \| | | | \___ \___ \
   ___) | |___ / ___ \| |\  | |_| |___) |__) |
  |____/ \____/_/   \_\_| \_|\___/|____/____/

  [!] No API key provided (--api-key)

  A subscription is required to use the SCANOSS API.
  Get yours at: https://www.scanoss.com
`

// normalizeURL trims surrounding space and any trailing slash, matching the SDK's
// own WithAPIURL handling, so endpoint comparisons are consistent.
func normalizeURL(u string) string {
	return strings.TrimRight(strings.TrimSpace(u), "/")
}

// checkAuth reads the api-url/api-key flags and applies the no-key guard. Call it
// at the top of a command's RunE to fail fast before doing any work.
func checkAuth(cmd *cobra.Command) error {
	apiURL, _ := cmd.Flags().GetString("api-url")
	apiKey, _ := cmd.Flags().GetString("api-key")
	return requireKeyForDefaultEndpoint(apiURL, apiKey)
}

// requireKeyForDefaultEndpoint fails fast (no request) when the user targets the
// default SCANOSS endpoint with no API key, printing the banner. A custom --api-url
// (e.g. an on-prem deployment) is exempt: it may legitimately run keyless.
func requireKeyForDefaultEndpoint(apiURL, apiKey string) error {
	if apiKey != "" {
		return nil
	}
	if normalizeURL(apiURL) != scanoss.DefaultAPIURL {
		return nil // custom endpoint → allow keyless (on-prem)
	}
	fmt.Fprint(os.Stderr, scanossNoKeyBanner)
	return errNoAPIKey
}

// renderAPIError prints a clear message for a 401 Unauthorized surfaced by the SDK
// transport, then returns the error unchanged (preserving the non-zero exit). Wrap
// the error returned by an SDK call with it so the user sees a useful hint instead
// of a bare "API returned status 401".
func renderAPIError(err error) error {
	if err == nil {
		return nil
	}
	var se *scanoss.StatusError
	if errors.As(err, &se) && se.StatusCode == http.StatusUnauthorized {
		fmt.Fprintln(os.Stderr, "Unauthorized: missing or invalid API key. Pass --api-key, or check your subscription at https://www.scanoss.com")
	}
	return err
}
