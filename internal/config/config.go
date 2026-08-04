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

package config

import "github.com/scanoss/scanoss.go/internal/version"

const (
	// DefaultAPIURL is the SCANOSS API endpoint the CLI targets unless --api-url
	// overrides it. The /v3 path is carried by the SDK service endpoints, so the
	// base URL is just the host.
	DefaultAPIURL = "https://api.scanoss.com"

	// DefaultThreads is the default number of parallel workers
	DefaultThreads = 10

	// Output format constants
	FormatRaw       = "raw"
	FormatSPDX      = "spdx"
	FormatCycloneDX = "cyclonedx"

	// DefaultFormat is the default output format
	DefaultFormat = FormatRaw

	// AppName is the application name used in SBOM metadata
	AppName = "scanoss"

	// OrganizationName is used in SBOM metadata
	OrganizationName = "SCANOSS"
)

// AppVersion is the application version, resolved at build time.
var AppVersion = version.Version()
