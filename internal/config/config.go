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

	// DefaultPostSize is the default maximum POST size in bytes (64KB)
	DefaultPostSize = 64 * 1024

	// MinFileSize is the minimum file size to process
	MinFileSize = 100

	// Output format constants
	FormatPlain     = "plain"
	FormatSPDX      = "spdx"
	FormatCycloneDX = "cyclonedx"

	// DefaultFormat is the default output format
	DefaultFormat = FormatPlain

	// AppName is the application name used in SBOM metadata
	AppName = "scanoss"

	// OrganizationName is used in SBOM metadata
	OrganizationName = "SCANOSS"
)

// AppVersion is the application version, resolved at build time.
var AppVersion = version.Version()

// Config contains the application configuration
type Config struct {
	// API settings
	APIURL string
	APIKey string

	// Scanning settings
	Threads  int
	PostSize int

	// Output settings
	OutputFile string

	// Processing settings
	TargetPath string
}

// NewConfig creates a new configuration with default values
func NewConfig() *Config {
	return &Config{
		APIURL:   DefaultAPIURL,
		Threads:  DefaultThreads,
		PostSize: DefaultPostSize,
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Threads < 1 {
		c.Threads = DefaultThreads
	}
	if c.PostSize < 1024 {
		c.PostSize = DefaultPostSize
	}
	if c.APIURL == "" {
		c.APIURL = DefaultAPIURL
	}
	return nil
}
