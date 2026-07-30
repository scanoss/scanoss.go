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

package scanoss_test

import (
	"context"
	"fmt"

	"github.com/scanoss/scanoss.go/pkg/scanoss"
)

// Basic usage: the chunking and worker pool are transparent — pass a list of
// PURLs to a service method and read the merged result.
func ExampleClient_Vulnerabilities() {
	client := scanoss.New(scanoss.WithAPIKey("YOUR_API_KEY"))

	purls := []string{
		"pkg:npm/lodash",
		"pkg:npm/express",
		"pkg:github/scanoss/engine",
	}

	res, err := client.Vulnerabilities.Components(context.Background(), scanoss.Components(purls...))
	if err != nil {
		panic(err)
	}

	// res is a typed *scanoss.VulnerabilitiesResponse (generated from the OpenAPI
	// v3 contract):
	fmt.Printf("%d components, status=%s\n", len(res.Components), res.Status.Status)
}

// Tuning chunk size and concurrency is done once at construction; the call site
// is unchanged.
func ExampleNew() {
	client := scanoss.New(
		scanoss.WithAPIKey("YOUR_API_KEY"),
		scanoss.WithChunkSize(20), // 20 PURLs per request
		scanoss.WithWorkers(10),   // up to 10 concurrent requests
	)

	comps := scanoss.Components("pkg:npm/lodash", "pkg:pypi/requests")

	// Each service is a single call; effective workers never exceed the chunk count.
	_, _ = client.Licenses.Attribution(context.Background(), comps)
	_, _ = client.Cryptography.Algorithms(context.Background(), comps)
	_, _ = client.Geoprovenance.Origins(context.Background(), comps)
}

// The pipeline runs a configurable set of decoration services in parallel over
// the same components, reports per-service progress, and returns one object
// keyed by service.
func ExampleClient_DecorationPipeline() {
	client := scanoss.New(scanoss.WithAPIKey("YOUR_API_KEY"), scanoss.WithWorkers(10))

	pipe := client.DecorationPipeline(
		scanoss.ServiceVulnerabilities,
		scanoss.ServiceLicenses,
	)
	pipe.Add(scanoss.ServiceCryptographyAlgorithms, scanoss.ServiceGeoprovenanceOrigin)

	// scanoss.Components turns a list of PURLs into the []Component input.
	// The reporter travels with the call: every update carries the service that produced it, so one
	// receiver renders them all.
	res, err := pipe.Run(context.Background(),
		scanoss.Components("pkg:npm/lodash", "pkg:pypi/requests"),
		scanoss.WithDecorationReporter(serviceRows{}))
	if err != nil {
		panic(err)
	}

	fmt.Println(res.String()) // {"vulnerabilities":{...}, "licenses":{...}, ...}
	for svc, e := range res.Errors {
		fmt.Printf("%s failed: %v\n", svc, e)
	}
}

// serviceRows renders one line per decoration update. Services run concurrently, so a real
// implementation would guard whatever it draws into.
type serviceRows struct{}

func (serviceRows) Decorating(service string, done, total int) {
	fmt.Printf("%-26s %d/%d purls\n", service, done, total)
}
