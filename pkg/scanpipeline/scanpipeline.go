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

// Package scanpipeline runs the scan pipeline that assembles a neutral sbom.Inventory. Run is
// the full flow over a source path: collect files → fingerprint → scan → source declared
// dependencies from the same files → gather + enrich the requested layers via the SDK's
// decoration pipeline (scanoss.DecorationPipeline). Build is the lower half (scan result →
// inventory) for callers that already have a scan result (e.g. a pre-generated WFP). Rendering
// is left to sbom.Generate — this package does not render. Layers to gather are driven by the
// caller's request, never by any output format.
package scanpipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	scanossapi "github.com/scanoss/scanoss.api-sdk"

	"github.com/scanoss/scanoss.go/pkg/dependencies"
	"github.com/scanoss/scanoss.go/pkg/dependencies/parsers"
	"github.com/scanoss/scanoss.go/pkg/filter"
	"github.com/scanoss/scanoss.go/pkg/sbom"
	"github.com/scanoss/scanoss.go/pkg/sbom/scansource"
	"github.com/scanoss/scanoss.go/pkg/scanner"
	"github.com/scanoss/scanoss.go/pkg/scanoss"
)

// Layer is an opt-in enrichment layer (requested via --include).
type Layer string

// Supported layers.
const (
	LayerDeps     Layer = "deps"
	LayerVulns    Layer = "vulns"
	LayerLicenses Layer = "licenses"
	LayerCrypto   Layer = "crypto"
	LayerGeo      Layer = "geo"
)

var known = map[Layer]bool{
	LayerDeps:     true,
	LayerVulns:    true,
	LayerLicenses: true,
	LayerCrypto:   true,
	LayerGeo:      true,
}

// Set is a set of requested layers.
type Set map[Layer]bool

// Has reports whether the layer was requested.
func (s Set) Has(l Layer) bool { return s[l] }

// ParseLayers validates a list of layer names (e.g. from --include) into a Set.
func ParseLayers(values []string) (Set, error) {
	set := make(Set, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		l := Layer(v)
		if !known[l] {
			return nil, fmt.Errorf("unknown --include layer %q (valid: deps, vulns, licenses, crypto, geo)", v)
		}
		set[l] = true
	}
	return set, nil
}

// Options configures Run, the full scan pipeline over a source path. Progress for the scan and
// each enrichment layer is reported through the client's own scanoss.WithProgress callback
// (keyed by Service); only fingerprinting — which happens before the API — needs OnFingerprint.
type Options struct {
	Client        *scanoss.Client       // required
	Layers        Set                   // requested output layers
	SourcePath    string                // file or directory to scan (required)
	Threads       int                   // fingerprint workers (<1 => 1)
	Filter        filter.Options        // file-collection filters (directory scans)
	ScanOptions   []scanoss.ScanOption  // per-scan tuning (chunk size, poll interval, BOM, ...)
	OnCollect     func(skipped int)     // optional: called once after collection with the filtered count
	OnFingerprint func(done, total int) // optional fingerprinting progress
}

// Result is the outcome of Run: the gathered inventory, the generated WFP (for --save-wfp), and
// the count of files that failed to fingerprint.
type Result struct {
	Inventory     sbom.Inventory
	WFP           []byte
	ProcessErrors int
}

// Run executes the full pipeline over Options.SourcePath: collect the files (applying the
// filters for a directory), fingerprint them, scan, source declared dependencies from the same
// file set (when the deps layer is requested), and gather + enrich into an Inventory. It owns
// everything from file collection onward; the caller supplies only flag-derived configuration.
func Run(ctx context.Context, opts Options) (Result, error) {
	info, err := os.Stat(opts.SourcePath)
	if err != nil {
		return Result{}, fmt.Errorf("accessing path: %w", err)
	}

	// Collect the files to scan: a directory is filtered, a single file is taken as-is. The
	// scan root labels the WFP paths relative to it. Fingerprinting and dependency scanning need
	// different filters: the fingerprint filter drops dependency manifests (package.json, go.mod,
	// …) since they aren't useful for matching, so when the deps layer is requested we collect a
	// second time with a dependency-scoped filter (same base rules, manifests preserved) and keep
	// only the manifests. The pipeline applies this second filter implicitly.
	var files []string
	var skipped int
	var manifestFiles []string
	scanRoot := opts.SourcePath
	if info.IsDir() {
		cr, collectErr := scanner.CollectFilesWithOptions(opts.SourcePath, opts.Filter)
		if collectErr != nil {
			return Result{}, fmt.Errorf("collecting files: %w", collectErr)
		}
		files, skipped = cr.Files, cr.SkippedCount

		if opts.Layers.Has(LayerDeps) {
			depFilter := opts.Filter
			depFilter.PreserveDependencyManifests = true
			dcr, depErr := scanner.CollectFilesWithOptions(opts.SourcePath, depFilter)
			if depErr != nil {
				return Result{}, fmt.Errorf("collecting dependency manifests: %w", depErr)
			}
			manifestFiles = dependencies.NewDependencyParser().FilterFiles(dcr.Files)
		}
	} else {
		abs, absErr := filepath.Abs(opts.SourcePath)
		if absErr != nil {
			abs = opts.SourcePath
		}
		files = []string{abs}
		scanRoot = filepath.Dir(opts.SourcePath)
		if opts.Layers.Has(LayerDeps) {
			manifestFiles = dependencies.NewDependencyParser().FilterFiles(files)
		}
	}
	if abs, absErr := filepath.Abs(scanRoot); absErr == nil {
		scanRoot = abs
	}
	if opts.OnCollect != nil {
		opts.OnCollect(skipped)
	}

	threads := opts.Threads
	if threads < 1 {
		threads = 1
	}

	// Run the two independent halves concurrently: the scan (fingerprint → upload → server) and
	// the declared-dependency resolution (parse manifests → resolve). They hit different API
	// endpoints and share no state, so there is no reason to serialize them. Enrichment waits for
	// both, since it decorates detected and declared components together.
	var (
		scanResult    *scanossapi.ScanResult
		wfp           []byte
		procErrors    int
		scanErr       error
		declaredComps []sbom.Component
		wg            sync.WaitGroup
	)

	wg.Add(1)
	go func() {
		defer wg.Done()
		w, errs := scanner.GenerateWFP(files, threads, scanRoot, opts.OnFingerprint)
		wfp, procErrors = w, len(errs)
		if len(w) == 0 {
			return
		}
		res, err := opts.Client.Scan.WFP(ctx, w, opts.ScanOptions...)
		if err != nil {
			scanErr = err
			return
		}
		if res.Result == nil {
			scanErr = fmt.Errorf("scan completed without a result")
			return
		}
		scanResult = res.Result
	}()

	// manifestFiles is populated only when the deps layer is requested (see the collection above).
	if len(manifestFiles) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			parsed, err := dependencies.NewDependencyParser().ParseFiles(manifestFiles)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: dependency scan failed: %v\n", err)
				return
			}
			declaredComps = resolveDeclared(ctx, opts.Client, parsed)
		}()
	}

	wg.Wait()
	if scanErr != nil {
		return Result{}, scanErr
	}
	if scanResult == nil && len(declaredComps) == 0 {
		return Result{ProcessErrors: procErrors}, nil
	}

	inv := assemble(ctx, opts.Client, scanResult, declaredComps, opts.Layers)
	return Result{Inventory: inv, WFP: wfp, ProcessErrors: procErrors}, nil
}

// Build sources an Inventory from a scan result and enriches it with the requested layers. It is
// the lower half of the pipeline (scan result → inventory), used directly by callers that
// already have a scan result. Scan matches populate the detected components; when the deps layer
// is requested and declared manifests are supplied, they are resolved into the same Components
// list, tagged declared. Every requested purl-layer (licenses, vulns, crypto, geo) is then
// gathered over all components, via the decoration pipeline. The requested layers, not any format,
// decide what is gathered. Enrichment is non-fatal: a failed service is logged and skipped so a
// partial inventory is still returned.
func Build(ctx context.Context, client *scanoss.Client, result *scanossapi.ScanResult, layers Set, declared *parsers.LocalDependencies) (sbom.Inventory, error) {
	// SOURCE the declared dependency components (opt-in via the deps layer). Resolving them is a
	// separate call because they start from a different input set (manifest-declared PURLs) than
	// the scan matches; once resolved they are ordinary components.
	var declaredComps []sbom.Component
	if layers.Has(LayerDeps) && declared != nil && len(declared.Files) > 0 {
		declaredComps = resolveDeclared(ctx, client, declared)
	}
	return assemble(ctx, client, result, declaredComps, layers), nil
}

// assemble merges the detected components (from the scan result) with the already-resolved
// declared components into one inventory, then enriches every component with the requested
// purl-layers. With no purl-layer requested the enrich step is a no-op — a bare scan does no
// decoration work.
func assemble(ctx context.Context, client *scanoss.Client, result *scanossapi.ScanResult, declaredComps []sbom.Component, layers Set) sbom.Inventory {
	inv := scansource.FromScanResult(result)
	inv.Components = append(inv.Components, declaredComps...)
	if len(inv.Components) > 0 {
		enrich(ctx, client, &inv, layers)
	}
	return inv
}

// enrich runs the decoration pipeline over the inventory's components and attaches the requested
// purl-layers in place — licenses/cryptography/geoprovenance inline on each component,
// vulnerabilities as the flat top-level list. Each layer is opt-in (driven by the requested set,
// never the output format); with no purl-layer requested it makes no API call.
func enrich(ctx context.Context, client *scanoss.Client, inv *sbom.Inventory, layers Set) {
	var services []scanoss.Service
	if layers.Has(LayerLicenses) {
		services = append(services, scanoss.ServiceLicenses)
	}
	if layers.Has(LayerVulns) {
		services = append(services, scanoss.ServiceVulnerabilities)
	}
	if layers.Has(LayerCrypto) {
		services = append(services, scanoss.ServiceCryptographyAlgorithms)
	}
	if layers.Has(LayerGeo) {
		services = append(services, scanoss.ServiceGeoprovenanceOrigin)
	}
	if len(services) == 0 {
		return
	}

	comps := make([]scanoss.Component, 0, len(inv.Components))
	for _, c := range inv.Components {
		comps = append(comps, scanoss.Component{Purl: c.Purl, Requirement: c.Version})
	}

	res, err := client.DecorationPipeline(services...).Run(ctx, comps)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: decoration pipeline failed: %v\n", err)
		return
	}
	for name, svcErr := range res.Errors {
		fmt.Fprintf(os.Stderr, "Warning: could not fetch %s: %v\n", name, svcErr)
	}

	// Per-component layers attach inline by PURL (+ version where the response echoes it).
	if r := res.Services[scanoss.ServiceLicenses.Name]; r != nil {
		if lr, decErr := scanoss.As[scanossapi.ComponentsLicenseResponse](r); decErr == nil {
			byKey := scansource.LicensesFrom(lr)
			for i := range inv.Components {
				c := &inv.Components[i]
				c.Licenses = byKey[scansource.LicenseKey(c.Purl, c.Version)]
			}
		}
	}
	if r := res.Services[scanoss.ServiceCryptographyAlgorithms.Name]; r != nil {
		if cr, decErr := scanoss.As[scanossapi.CryptoAlgorithmsResponse](r); decErr == nil {
			byKey := scansource.CryptographyFrom(cr)
			for i := range inv.Components {
				c := &inv.Components[i]
				c.Cryptography = byKey[scansource.LicenseKey(c.Purl, c.Version)]
			}
		}
	}
	if r := res.Services[scanoss.ServiceGeoprovenanceOrigin.Name]; r != nil {
		if gr, decErr := scanoss.As[scanossapi.GeoOriginResponse](r); decErr == nil {
			byPurl := scansource.GeoprovenanceFrom(gr)
			for i := range inv.Components {
				c := &inv.Components[i]
				c.Geoprovenance = byPurl[c.Purl]
			}
		}
	}

	// Vulnerabilities → flat top-level list joined by base PURL.
	if r := res.Services[scanoss.ServiceVulnerabilities.Name]; r != nil {
		if vr, decErr := scanoss.As[scanossapi.VulnerabilitiesResponse](r); decErr == nil {
			inv.Vulnerabilities = scansource.VulnerabilitiesFrom(vr)
		}
	}
}

// resolveDeclared resolves the declared dependencies parsed from manifests into ScopeDeclared
// components, joining each back to its manifest origin by PURL. They then enrich alongside the
// scan matches. Resolution is non-fatal: a failure is logged and yields no declared components.
func resolveDeclared(ctx context.Context, client *scanoss.Client, declared *parsers.LocalDependencies) []sbom.Component {
	var comps []scanoss.Component
	declaredIn := make(map[string]string)
	for _, file := range declared.Files {
		for _, p := range file.Purls {
			comps = append(comps, scanoss.Component{Purl: p.Purl, Requirement: p.Requirement})
			if _, ok := declaredIn[p.Purl]; !ok {
				declaredIn[p.Purl] = file.File
			}
		}
	}

	resp, err := client.Dependencies.Dependencies(ctx, comps)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not resolve declared dependencies: %v\n", err)
		return nil
	}

	deps := scansource.DeclaredFrom(resp)
	for i := range deps {
		deps[i].DeclaredIn = declaredIn[deps[i].Purl]
	}
	return deps
}
