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

// Options configures Run, the full scan pipeline over a source path. Every layer reports through
// OnProgress — the steps the pipeline runs itself and the ones it delegates to the SDK alike, so
// a caller listens on one channel rather than reconciling two.
type Options struct {
	Client *scanoss.Client // required

	// Services are the decoration services to gather over the components. The caller resolves
	// these — from a flag, a config file, or nothing at all — and hands over the result: this
	// package knows services, not whatever vocabulary produced them.
	Services []scanoss.Service

	// SourceDeclared asks for declared dependencies to be sourced from the dependency manifests in
	// the same tree, and merged into the inventory alongside the scan's detected components. It is
	// not a decoration service: nothing is fetched, the manifests already carry resolved PURLs.
	SourceDeclared bool

	SourcePath string         // file or directory to scan (required)
	Threads    int            // fingerprint workers (<1 => 1)
	Filter     filter.Options // file-collection filters (directory scans)
	// DependencySettings is the scanoss.json skip rules for the dependencies
	// operation. The manifest collection is a stage of its own, with its own
	// profile, so it cannot reuse Filter.Settings (which holds the scanning
	// section). Nil when there is no scanoss.json.
	DependencySettings *filter.Settings
	ScanOptions        []scanoss.ScanOption // per-scan tuning (chunk size, poll interval, BOM, ...)

	// OnProgress receives every layer's progress. Optional; nil reports nothing. The pipeline runs
	// layers concurrently, so it must be safe for concurrent use.
	OnProgress func(Progress)
}

// Result is the outcome of Run: the gathered inventory, the generated WFP (for --save-wfp) and the
// count of files that failed to fingerprint.
//
// What the filters excluded is not here: it is reported as the collect layer completing, while the
// scan is still ahead, rather than handed back once everything is over.
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
	// Checked here rather than left to the first dereference: the client is used inside the scan
	// goroutine below, where a nil panics on a stack the caller cannot recover from, and Run's own
	// deferred Wait would hand back a nil error while the process is already being torn down.
	if opts.Client == nil {
		return Result{}, fmt.Errorf("a client is required (Options.Client)")
	}

	r := NewReporter(opts.OnProgress)
	emit := r.emit

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

		if opts.SourceDeclared {
			// Collecting manifests is its own stage, so it uses the dependency
			// profile rather than inheriting the scan's. Only what the user asked
			// for carries over; the profile decides the rest, which is what keeps
			// this in step with the standalone `dependencies` command.
			depFilter := filter.DependencyOptions()
			depFilter.MinSize = opts.Filter.MinSize
			depFilter.MaxSize = opts.Filter.MaxSize
			depFilter.FolderDefaults = opts.Filter.FolderDefaults
			depFilter.FileDefaults = opts.Filter.FileDefaults
			depFilter.Settings = opts.DependencySettings
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
		if opts.SourceDeclared {
			manifestFiles = dependencies.NewDependencyParser().FilterFiles(files)
		}
	}
	if abs, absErr := filepath.Abs(scanRoot); absErr == nil {
		scanRoot = abs
	}
	// The walk has no incremental progress — its total is unknown until it ends — so it reports
	// once, on completion: how many files it kept out of how many it saw. A consumer wanting the
	// excluded count subtracts.
	emit(Progress{Layer: LayerCollect, Status: StatusCompleted, Done: len(files), Total: len(files) + skipped})

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

		// Fingerprinting stays here rather than going through Scan.Files, which would do it too:
		// this package already has the file list, the root and the worker count as plain values,
		// and handing them to the SDK instead would mean three scan options existing for no reason
		// but to carry them back. It reports the stage itself and hands over a finished WFP.
		w, errs := scanner.GenerateWFP(files, threads, scanRoot, r.Fingerprinting)
		wfp, procErrors = w, len(errs)
		emit(Progress{Layer: LayerFingerprint, Status: StatusCompleted, Done: len(files), Total: len(files)})
		if len(w) == 0 {
			return
		}

		// The reporter travels with the call, not with the client: the client is the caller's, and a
		// reporter it registered for itself keeps working untouched.
		// A fresh slice rather than appending to the caller's: append writes into their array
		// whenever it has spare capacity, so the caller's Options would sprout an option it never
		// asked for. Options is exported, so the slice is theirs to reuse.
		scanOpts := make([]scanoss.ScanOption, 0, len(opts.ScanOptions)+1)
		scanOpts = append(scanOpts, opts.ScanOptions...)
		scanOpts = append(scanOpts, scanoss.WithScanReporter(r))

		res, err := opts.Client.Scan.WFP(ctx, w, scanOpts...)
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
			total := len(manifestFiles)
			emit(Progress{Layer: LayerManifests, Status: StatusRunning, Total: total}) // show it up front, alongside fingerprinting
			parsed, err := dependencies.NewDependencyParser().ParseFiles(manifestFiles)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: dependency scan failed: %v\n", err)
				emit(Progress{Layer: LayerManifests, Status: StatusFailed, Total: total})
				return
			}
			declaredComps = sourceDeclared(parsed, scanRoot)
			emit(Progress{Layer: LayerManifests, Status: StatusCompleted, Done: total, Total: total})
		}()
	}

	wg.Wait()
	if scanErr != nil {
		return Result{}, scanErr
	}
	if scanResult == nil && len(declaredComps) == 0 {
		return Result{ProcessErrors: procErrors}, nil
	}

	// assemble runs the enrichment layers, which report to r as well.
	inv := assemble(ctx, opts.Client, scanResult, declaredComps, opts.Services, r)
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
func Build(ctx context.Context, client *scanoss.Client, result *scanossapi.ScanResult, services []scanoss.Service, includeDeclared bool, declared *parsers.LocalDependencies, reporter scanoss.DecorationReporter) (sbom.Inventory, error) {
	// SOURCE the declared dependency components (opt-in via the deps layer) straight from the parsed
	// manifests — no dependency-service round trip, since the lockfile already carries resolved
	// PURLs. Once sourced they are ordinary components, enriched alongside the scan matches.
	var declaredComps []sbom.Component
	if includeDeclared && declared != nil && len(declared.Files) > 0 {
		declaredComps = sourceDeclared(declared, "")
	}
	return assemble(ctx, client, result, declaredComps, services, reporter), nil
}

// assemble merges the detected components (from the scan result) with the already-resolved
// declared components into one inventory, then enriches every component with the requested
// purl-layers. With no purl-layer requested the enrich step is a no-op — a bare scan does no
// decoration work.
func assemble(ctx context.Context, client *scanoss.Client, result *scanossapi.ScanResult, declaredComps []sbom.Component, services []scanoss.Service, r scanoss.DecorationReporter) sbom.Inventory {
	inv := scansource.Inventory(result)
	inv.Components = append(inv.Components, declaredComps...)
	inv.Components = dedupeComponents(inv.Components)
	if len(inv.Components) > 0 {
		Enrich(ctx, client, &inv, services, r)
	}
	return inv
}

// dedupeComponents collapses components that share the same identity (PURL + version) into one.
// The scan and the manifests routinely surface the same component — a package listed in both
// package.json and package-lock.json, or a detected match that is also a declared dependency —
// and left in place the duplicates repeat in the raw output and, worse, produce clashing SBOM
// identifiers (SPDX SPDXID / CycloneDX bom-ref are keyed by PURL+version), making the document
// invalid. Different versions of the same PURL are kept (distinct identity).
func dedupeComponents(comps []sbom.Component) []sbom.Component {
	index := make(map[string]int, len(comps))
	out := make([]sbom.Component, 0, len(comps))
	for _, c := range comps {
		key := c.Purl + "@" + c.Version
		if i, ok := index[key]; ok {
			mergeComponent(&out[i], c)
			continue
		}
		index[key] = len(out)
		out = append(out, c)
	}
	return out
}

// mergeComponent folds src into dst (same identity): a detected scope wins over declared, and the
// evidence lists are combined — so a component that is both scan-detected and manifest-declared
// keeps its file matches and its manifest occurrence together.
func mergeComponent(dst *sbom.Component, src sbom.Component) {
	if dst.Scope == sbom.ScopeDeclared && src.Scope == sbom.ScopeDetected {
		dst.Scope = sbom.ScopeDetected
	}
	dst.Evidence = addEvidence(dst.Evidence, src.Evidence...)
}

// Enrich runs the decoration pipeline over the inventory's components and attaches the requested
// purl-layers in place — licenses/cryptography/geoprovenance inline on each component,
// vulnerabilities as the flat top-level list. It is the pipeline's format-blind enrichment stage,
// keyed purely by PURL (+ version): the scan path reaches it through Build/Run, and the enrich
// command calls it directly on an inventory parsed from an existing SBOM — no scan required. Each
// layer is opt-in (driven by the requested set, never the output format); with no purl-layer
// requested it makes no API call. Enrichment is non-fatal: a failed service is logged and skipped.
func Enrich(ctx context.Context, client *scanoss.Client, inv *sbom.Inventory, services []scanoss.Service, reporter scanoss.DecorationReporter) {
	if len(services) == 0 {
		return
	}

	comps := make([]scanoss.Component, 0, len(inv.Components))
	for _, c := range inv.Components {
		comps = append(comps, scanoss.Component{Purl: c.Purl, Requirement: c.Version})
	}

	// Announce every layer before starting, so each is on screen from the outset rather than
	// appearing when its first response lands. The services run concurrently but answer at very
	// different speeds — one endpoint can take ten times longer per request than another — and a
	// layer that only shows up once it has an answer looks like a layer that started late.
	if reporter != nil {
		for _, svc := range services {
			reporter.Decorating(svc.Name, 0, len(comps))
		}
	}

	res, err := client.DecorationPipeline(services...).Run(ctx, comps, scanoss.WithDecorationReporter(reporter))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: decoration pipeline failed: %v\n", err)
		return
	}
	for name, svcErr := range res.Errors {
		fmt.Fprintf(os.Stderr, "Warning: could not fetch %s: %v\n", name, svcErr)
	}

	// Per-component layers attach inline by PURL (+ version where the response echoes it).
	if lic := res.Licenses; lic != nil {
		warnPartial(scanoss.ServiceLicenses.Name, lic.Failed)
		byKey := scansource.Licenses(lic.Response)
		for i := range inv.Components {
			c := &inv.Components[i]
			c.Licenses = byKey[scansource.Key(c.Purl, c.Version)]
		}
	}
	if cry := res.Cryptography; cry != nil {
		warnPartial(scanoss.ServiceCryptographyAlgorithms.Name, cry.Failed)
		byKey := scansource.Cryptography(cry.Response)
		for i := range inv.Components {
			c := &inv.Components[i]
			c.Cryptography = byKey[scansource.Key(c.Purl, c.Version)]
		}
	}
	if geo := res.Geoprovenance; geo != nil {
		warnPartial(scanoss.ServiceGeoprovenanceOrigin.Name, geo.Failed)
		byPurl := scansource.Geoprovenance(geo.Response)
		for i := range inv.Components {
			c := &inv.Components[i]
			c.Geoprovenance = byPurl[c.Purl]
		}
	}

	// Vulnerabilities → flat top-level list joined by base PURL.
	if vul := res.Vulnerabilities; vul != nil {
		warnPartial(scanoss.ServiceVulnerabilities.Name, vul.Failed)
		inv.Vulnerabilities = scansource.Vulnerabilities(vul.Response)
	}
}

// warnPartial reports a layer that arrived incomplete, naming the components it left out.
// Without it the gap is invisible: a component missing from a partial response is
// indistinguishable from one the service had nothing to say about.
func warnPartial(svc string, failed []scanoss.ChunkError) {
	if len(failed) == 0 {
		return
	}
	var purls []string
	for _, f := range failed {
		purls = append(purls, f.Purls...)
	}
	// A long list is truncated: the point is to name what is missing, not to fill the terminal
	// when a whole scan's worth of components went unanswered.
	shown := purls
	suffix := ""
	if len(shown) > 5 {
		shown, suffix = shown[:5], fmt.Sprintf(" (and %d more)", len(purls)-5)
	}
	fmt.Fprintf(os.Stderr, "Warning: %s has no data for %d component(s): %s%s\n",
		svc, len(purls), strings.Join(shown, ", "), suffix)
}

// sourceDeclared builds ScopeDeclared components directly from the manifests parsed from the
// project — no dependency-service round trip. The parser already yields resolved PURLs (pinned in
// the lockfile), so we only split each `pkg:...@version` into its base PURL and version, tag it
// declared, and record its manifest origin. The decoration pipeline then adds the requested
// purl-layers (licenses, vulns, …) over these alongside the scan matches. Duplicates are removed
// (see dedupeDeclared).
func sourceDeclared(declared *parsers.LocalDependencies, scanRoot string) []sbom.Component {
	if declared == nil {
		return nil
	}
	var comps []sbom.Component
	for _, file := range declared.Files {
		manifest := relativeTo(file.File, scanRoot) // same project-relative form as scan-match evidence
		for _, p := range file.Purls {
			purl, version := splitPurlVersion(p.Purl)
			if version == "" {
				version = p.Requirement
			}
			if purl == "" {
				continue
			}
			comps = append(comps, sbom.Component{
				Purl:     purl,
				Version:  version,
				Scope:    sbom.ScopeDeclared,
				Evidence: []sbom.FileEvidence{{Path: manifest, MatchType: "declared"}},
			})
		}
	}
	return dedupeDeclared(comps)
}

// relativeTo returns path relative to root when path is under it, so a declared dependency's
// manifest evidence uses the same project-relative path as scan-match evidence. Falls back to the
// path unchanged (root empty, or path outside root).
func relativeTo(path, root string) string {
	if root == "" {
		return path
	}
	if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return path
}

// splitPurlVersion splits a PURL into its base identity and version, e.g.
// "pkg:npm/@scope/name@1.2.3" -> ("pkg:npm/@scope/name", "1.2.3"). The version separator is the
// last "@" after the final "/", so a scoped-npm "@scope" prefix (which has no preceding "/") is
// never mistaken for a version.
func splitPurlVersion(purl string) (base, version string) {
	slash := strings.LastIndex(purl, "/")
	if at := strings.LastIndex(purl, "@"); at > slash {
		return purl[:at], purl[at+1:]
	}
	return purl, ""
}

// dedupeDeclared collapses exact duplicate declared components — the same package at the same
// version listed more than once (across manifest sections, or in both package.json and its
// lockfile). Their manifest evidence is merged, so a package declared in several manifests keeps
// one occurrence per manifest. Distinct versions of a package are kept, including a declared range
// (package.json "^1.5.5") alongside its resolved pin (package-lock.json "1.14.3") — those are
// different, meaningful entries, not duplicates.
func dedupeDeclared(comps []sbom.Component) []sbom.Component {
	index := make(map[string]int, len(comps))
	out := make([]sbom.Component, 0, len(comps))
	for _, c := range comps {
		key := c.Purl + "@" + c.Version
		if i, ok := index[key]; ok {
			out[i].Evidence = addEvidence(out[i].Evidence, c.Evidence...)
			continue
		}
		index[key] = len(out)
		out = append(out, c)
	}
	return out
}

// addEvidence appends the evidence entries not already present (by path + match type), so merging
// duplicate or overlapping components keeps one occurrence per distinct origin.
func addEvidence(dst []sbom.FileEvidence, add ...sbom.FileEvidence) []sbom.FileEvidence {
	for _, e := range add {
		dup := false
		for _, existing := range dst {
			if existing.Path == e.Path && existing.MatchType == e.MatchType {
				dup = true
				break
			}
		}
		if !dup {
			dst = append(dst, e)
		}
	}
	return dst
}
