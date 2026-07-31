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
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/scanoss/scanoss.go/internal/cliconfig"
	"github.com/scanoss/scanoss.go/pkg/scanoss"
)

// addAPIFlags registers the flags every command that talks to the SCANOSS API
// needs: where to reach it, how to authenticate, how to get there, and where to
// write the result. It is their single declaration — a new one is added here and
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
	fs.String("proxy", "", "Proxy URL, e.g. http://proxy.example.com:8080 (also honors HTTP_PROXY/HTTPS_PROXY)")
	fs.String("ca-cert", "", "Path to a PEM file with an additional CA to trust")
	fs.Bool("ignore-cert-errors", false, "Ignore TLS certificate errors (insecure)")
	fs.StringP("output", "o", "", "Output file (default: stdout)")
}

// apiConfig resolves the five flags addAPIFlags gives every API command — endpoint, key,
// proxy, CA file and ignore-cert-errors — into the Config that carries them. Commands add
// their own fields on top; nothing else reads these flags, and no command builds an
// *http.Client of its own.
//
// chunk-size and workers are deliberately absent: chunk-size means PURLs per request on
// the purl commands and upload bytes on scan, so resolving it here would misconfigure one
// of them.
//
// With no flags set the SDK keeps Go's defaults, which means HTTP_PROXY, HTTPS_PROXY and
// NO_PROXY are honoured without any code of ours.
func apiConfig(cmd *cobra.Command) (scanoss.Config, error) {
	api, err := cliconfig.ResolveAPI(cmd.Flags())
	if err != nil {
		return scanoss.Config{}, err
	}
	// Resolved rather than read off the flags: both can also come from the
	// environment or ~/.scanoss/settings.json, so a stored value works with no flag.
	transport, err := cliconfig.ResolveTransport(cmd.Flags())
	if err != nil {
		return scanoss.Config{}, err
	}
	// ignore-cert-errors stays a flag. Storing "never verify certificates" would
	// remove the deliberateness that makes it acceptable per run.
	insecure, _ := cmd.Flags().GetBool("ignore-cert-errors")

	if insecure {
		slog.Warn("ignoring TLS certificate errors (insecure)")
		// Dropped, not just unused: saying the CA has no effect and then failing
		// because that same file is unreadable would contradict itself. With
		// verification off the file is never consulted, so it is not read either.
		if transport.CACertFile != "" {
			slog.Warn("ca-cert has no effect with --ignore-cert-errors")
			transport.CACertFile = ""
		}
	}

	return scanoss.Config{
		APIURL:      api.URL,
		APIKey:      api.Key,
		Proxy:       transport.Proxy,
		CACertFile:  transport.CACertFile,
		InsecureTLS: insecure,
	}, nil
}
