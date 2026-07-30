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

package scanoss

import scanossapi "github.com/scanoss/scanoss.api-sdk"

// ScanReporter receives the stages of scanning a source tree, one method per stage. Each reports
// exactly what that stage has — the local ones a count, the server one its envelope — so no
// update's meaning depends on which other field happens to be set.
//
// Which stages run depends on the entry point: Scan.WFP is handed its fingerprints already made and
// never reports Fingerprinting, and Scan.Wait resumes an uploaded scan, so it only reports
// Scanning. A stage that does not run simply never calls.
//
// Register an implementation with WithScanReporter. Methods are called from whichever goroutine
// reached that stage — Uploading from the upload workers, so from several — but never two at once:
// the SDK serialises them, and each call happens before the next. An implementation therefore needs
// no lock of its own. It must not block, since a slow reporter holds up the stage reporting it.
type ScanReporter interface {
	// Fingerprinting reports local hashing as files are hashed. done only ever grows.
	Fingerprinting(done, total int)

	// Uploading reports WFP blocks handed to the server. done only ever grows: it counts 1..total,
	// one call per block, and the last call carries total.
	Uploading(done, total int)

	// Scanning reports one status poll of a running scan, handing over the server's envelope
	// verbatim: the SDK does not choose which of its fields matter.
	//
	// The server scans in passes ("Pass 1: scan files", "Pass 2: scan snippets", ...) that each
	// restart PhaseDone/PhaseTotal and count something different from the last, so those counters
	// are comparable only within one Phase. A caller rendering them as a single bar must segment it
	// by Phase; feeding them straight to one bar makes it run backwards.
	//
	// Polling samples the scan rather than streaming it, so a pass shorter than the poll interval
	// is never reported at all. Whether the scan succeeded is Status, never these counters.
	Scanning(env scanossapi.ScanEnvelope)
}

// DecorationReporter receives enrichment progress: the components being gathered over, by service.
//
// It is separate from ScanReporter because decoration needs no scan — an inventory parsed from a
// file is enriched the same way — and because there are consumers of each half alone: a command
// that only queries PURLs implements this and nothing else.
type DecorationReporter interface {
	// Decorating reports one service's advance over the components, counted in PURLs. Services run
	// concurrently, so updates from several arrive interleaved, each tagged with its own name
	// (Service.Name, e.g. "licenses" or "cryptography.algorithms").
	Decorating(service string, done, total int)
}

// WithScanReporter reports this scan's stages to r. Optional; by default the SDK reports nothing.
//
// It belongs to the call rather than to the Client on purpose: a Client is long-lived and shared —
// a caller may hand the same one to several operations, or to a library — while an observer belongs
// to the one operation it is watching.
func WithScanReporter(r ScanReporter) ScanOption {
	return func(o *scanOptions) { o.reporter = r }
}

// DecorateOption configures a single decoration call. It is the decoration counterpart of
// ScanOption.
type DecorateOption func(*decorateOptions)

type decorateOptions struct {
	reporter DecorationReporter // receives this call's advance (WithDecorationReporter)
}

func resolveDecorateOptions(opts []DecorateOption) decorateOptions {
	var o decorateOptions
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// WithDecorationReporter reports this call's advance to r, service by service. Optional; by default
// the SDK reports nothing. Services run concurrently, so r must be safe for concurrent use.
func WithDecorationReporter(r DecorationReporter) DecorateOption {
	return func(o *decorateOptions) { o.reporter = r }
}
