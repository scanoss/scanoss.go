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

import (
	"context"
	"fmt"

	scanossapi "github.com/scanoss/scanoss.api-sdk"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/scanoss/scanoss.go/pkg/filter"
	"github.com/scanoss/scanoss.go/pkg/scanner"
	"github.com/scanoss/scanoss.go/pkg/settings"
)

// ScanAPI is the batch scan service surface. Folder, Files and WFP each run the
// full flow — upload (parallel byte-range chunks) + poll to completion — and
// return the envelope with its Result populated. They differ only in the input:
// a directory tree, an explicit file list, or an already-assembled WFP. The
// caller never manages the scan id; stages are reported via WithScanReporter and
// the client-generated id is surfaced via WithScanIDNotify for optional recovery.
type ScanAPI interface {
	// Folder collects, fingerprints and scans a directory tree (or single file).
	Folder(ctx context.Context, path string, opts ...ScanOption) (scanossapi.ScanEnvelope, error)

	// Files fingerprints and scans an explicit list of files.
	Files(ctx context.Context, files []string, opts ...ScanOption) (scanossapi.ScanEnvelope, error)

	// WFP scans an already-assembled WFP byte stream.
	WFP(ctx context.Context, wfp []byte, opts ...ScanOption) (scanossapi.ScanEnvelope, error)

	// Status performs a single status poll for a known scan id.
	Status(ctx context.Context, scanID string) (scanossapi.ScanEnvelope, error)

	// Wait resumes polling a known scan id until a terminal state. Used to recover
	// an interrupted scan (e.g. `scanoss-cli results <id>`). The poll cadence can be
	// tuned with WithPollInterval.
	Wait(ctx context.Context, scanID string, opts ...ScanOption) (scanossapi.ScanEnvelope, error)
}

// Scan defaults applied when the corresponding option is not given.
// DefaultScanChunkBytes is the WFP upload block size (1 MiB).
const DefaultScanChunkBytes = 1 << 20

// ScanOption configures a single scan (Folder / Files / WFP).
type ScanOption func(*scanOptions)

type scanOptions struct {
	chunkBytes   int            // WFP upload block size
	filters      filter.Options // file collection filters (Folder only)
	root         string         // when set, WFP file paths are rewritten relative to it
	bom          *settings.BOM  // when set, BOM rules are applied to the result (post-scan)
	pollInterval time.Duration  // scan status poll cadence (WithPollInterval)
	reporter     ScanReporter   // receives this scan's stages (WithScanReporter)
}

func resolveScanOptions(opts []ScanOption) scanOptions {
	o := scanOptions{
		chunkBytes: DefaultScanChunkBytes,
		// The profile, not a hand-built equivalent: the two agree today only because the default
		// size bounds happen to be an int's zero value, so a change to either would silently leave
		// a scan filtering differently from everything else that uses the scanning profile.
		filters:      filter.ScanOptions(),
		pollInterval: DefaultScanPollInterval,
	}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// WithChunkBytes sets the WFP upload block size in bytes (default
// DefaultScanChunkBytes). Values <= 0 are ignored.
func WithChunkBytes(n int) ScanOption {
	return func(o *scanOptions) {
		if n > 0 {
			o.chunkBytes = n
		}
	}
}

// WithFilters sets the file-collection filters used by Folder (default skip
// lists, .gitignore, size bounds, scanoss.json rules).
func WithFilters(f filter.Options) ScanOption {
	return func(o *scanOptions) { o.filters = f }
}

// WithBOM applies the scan's bill-of-materials rules to the result. Today it runs
// bom.remove (with bom.include as precedence) post-scan: matching file matches are
// neutralized and unreferenced components pruned. The whole BOM is passed so future
// behaviors (pre-scan include/identify context, post-scan replace) extend this same
// option. A nil BOM is a no-op.
func WithBOM(bom *settings.BOM) ScanOption {
	return func(o *scanOptions) { o.bom = bom }
}

// WithPollInterval sets how often the scan status endpoint is polled while waiting
// for a scan to finish (default DefaultScanPollInterval). Values <= 0 are ignored.
// Very small intervals increase load on the server. Applies to the full scan flow
// (Folder/Files/WFP) and to Wait when resuming a known scan id.
func WithPollInterval(d time.Duration) ScanOption {
	return func(o *scanOptions) {
		if d > 0 {
			o.pollInterval = d
		}
	}
}

type scanService struct{ c *Client }

var _ ScanAPI = scanService{}

// Folder collects files under path (applying the filter options), fingerprints them,
// then scans the resulting WFP.
func (s scanService) Folder(ctx context.Context, path string, opts ...ScanOption) (scanossapi.ScanEnvelope, error) {
	o := resolveScanOptions(opts)
	if o.root == "" {
		o.root = path // report result paths relative to the scanned folder
	}
	res, err := scanner.CollectFilesWithOptions(path, o.filters)
	if err != nil {
		return scanossapi.ScanEnvelope{}, fmt.Errorf("error collecting files: %w", err)
	}
	wfp, err := s.fingerprint(res.Files, o)
	if err != nil {
		return scanossapi.ScanEnvelope{}, err
	}
	return s.scan(ctx, wfp, o)
}

// Files fingerprints an explicit list of files, then scans the resulting WFP.
func (s scanService) Files(ctx context.Context, files []string, opts ...ScanOption) (scanossapi.ScanEnvelope, error) {
	o := resolveScanOptions(opts)
	wfp, err := s.fingerprint(files, o)
	if err != nil {
		return scanossapi.ScanEnvelope{}, err
	}
	return s.scan(ctx, wfp, o)
}

// WFP scans an already-assembled WFP byte stream.
func (s scanService) WFP(ctx context.Context, wfp []byte, opts ...ScanOption) (scanossapi.ScanEnvelope, error) {
	return s.scan(ctx, wfp, resolveScanOptions(opts))
}

// scan uploads a ready WFP (parallel chunks) and waits for completion. The three entry
// points (Folder/Files/WFP) each produce the WFP, then funnel through here.
func (s scanService) scan(ctx context.Context, wfp []byte, o scanOptions) (scanossapi.ScanEnvelope, error) {
	s.c.log.Debug("scan: uploading WFP", "bytes", len(wfp), "chunkBytes", o.chunkBytes)
	scanID, err := s.upload(ctx, wfp, o.chunkBytes, o.reporter)
	if err != nil {
		return scanossapi.ScanEnvelope{}, err
	}
	s.c.log.Info("scan uploaded", "scanID", scanID)

	env, err := s.wait(ctx, scanID, o.pollInterval, o.reporter)
	if err != nil {
		return scanossapi.ScanEnvelope{}, err
	}
	// Apply BOM rules on the result (post-scan)
	if o.bom != nil && env.Result != nil {
		s.c.log.Debug("applying BOM rules to scan result")
		ApplyBOMRemove(env.Result, o.bom)
	}
	s.c.log.Info("scan complete", "scanID", scanID)
	return env, nil
}

// fingerprint runs the worker pool over files and returns the combined WFP,
// emitting per-file progress. Bounded by the client's worker limit.
func (s scanService) fingerprint(files []string, o scanOptions) ([]byte, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no files to scan")
	}
	wfp, _ := scanner.GenerateWFP(files, s.c.workers, o.root, func(done, total int) {
		if o.reporter != nil {
			o.reporter.Fingerprinting(done, total)
		}
	})
	s.c.log.Debug("fingerprinted files", "files", len(files), "wfpBytes", len(wfp))
	if len(wfp) == 0 {
		return nil, fmt.Errorf("no fingerprints generated")
	}
	return wfp, nil
}

// upload generates the scan id (UUIDv7) client-side, uploads every WFP chunk
// concurrently (each carrying the id, bounded by the client's worker limit), then
// fires the notify hook. The hook fires only after the full WFP is on the server,
// so the surfaced id is always resumable. It returns the scan id.
func (s scanService) upload(ctx context.Context, wfp []byte, chunkBytes int, r ScanReporter) (string, error) {
	if len(wfp) == 0 {
		return "", fmt.Errorf("no fingerprints to scan")
	}

	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("error generating scan id: %w", err)
	}
	scanID := id.String()

	ranges := chunkRanges(len(wfp), chunkBytes)
	s.c.log.Debug("uploading WFP chunks", "scanID", scanID, "chunks", len(ranges))
	prog := &chunkProgress{r: r, total: len(ranges)}
	if err := s.uploadChunks(ctx, scanID, wfp, ranges, prog); err != nil {
		return "", err
	}

	// The full WFP is now on the server: the id is resumable. Surface it before
	// polling so an interrupted scan can be recovered with `results <id>`.
	if s.c.onScanID != nil {
		s.c.onScanID(scanID)
	}
	return scanID, nil
}

// uploadChunks uploads all chunks concurrently, bounded by the client's worker
// limit, aborting all on the first error.
func (s scanService) uploadChunks(ctx context.Context, scanID string, wfp []byte, ranges [][2]int, prog *chunkProgress) error {
	if len(ranges) == 0 {
		return nil
	}

	workers := s.c.workers
	if len(ranges) < workers {
		workers = len(ranges)
	}
	if workers < 1 {
		workers = 1
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan [2]int)
	errCh := make(chan error, len(ranges))
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range jobs {
				select {
				case <-ctx.Done():
					errCh <- ctx.Err()
					continue
				default:
				}
				if err := s.c.uploadChunk(ctx, scanID, r[0], r[1], len(wfp), blockOf(wfp, r)); err != nil {
					errCh <- err
					cancel()
					continue
				}
				prog.inc()
			}
		}()
	}
	for _, r := range ranges {
		jobs <- r
	}
	close(jobs)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

// blockOf returns the WFP byte slice for the inclusive range r = [off, end].
func blockOf(wfp []byte, r [2]int) []byte { return wfp[r[0] : r[1]+1] }

// chunkProgress reports upload progress in chunks. Safe for concurrent inc.
type chunkProgress struct {
	r     ScanReporter
	done  int64
	total int
}

func (p *chunkProgress) inc() {
	atomic.AddInt64(&p.done, 1)
	if p.r != nil {
		p.r.Uploading(int(atomic.LoadInt64(&p.done)), p.total)
	}
}

// Status performs a single status poll.
func (s scanService) Status(ctx context.Context, scanID string) (scanossapi.ScanEnvelope, error) {
	if scanID == "" {
		return scanossapi.ScanEnvelope{}, fmt.Errorf("no scan id")
	}
	pollURL := s.c.apiURL + ServiceScan.endpoint + "/" + scanID
	req, err := http.NewRequest(http.MethodGet, pollURL, nil)
	if err != nil {
		return scanossapi.ScanEnvelope{}, fmt.Errorf("error creating request: %w", err)
	}
	body, _, err := s.c.transport.do(ctx, req)
	if err != nil {
		return scanossapi.ScanEnvelope{}, err
	}
	return parseScanEnvelope(body)
}

// Wait polls scanID until it reaches a terminal state, then returns the envelope
// (with its Result populated). It honors ctx cancellation without cancelling the
// server-side scan. The poll cadence defaults to DefaultScanPollInterval and can
// be overridden with WithPollInterval.
func (s scanService) Wait(ctx context.Context, scanID string, opts ...ScanOption) (scanossapi.ScanEnvelope, error) {
	o := resolveScanOptions(opts)
	return s.wait(ctx, scanID, o.pollInterval, o.reporter)
}

// wait is the polling loop shared by the full scan flow and the public Wait. The
// first poll fires after scanPollInitial, clamped to interval so a sub-second
// cadence stays responsive.
func (s scanService) wait(ctx context.Context, scanID string, interval time.Duration, r ScanReporter) (scanossapi.ScanEnvelope, error) {
	if scanID == "" {
		return scanossapi.ScanEnvelope{}, fmt.Errorf("no scan id")
	}

	initial := scanPollInitial
	if interval < initial {
		initial = interval
	}
	select {
	case <-time.After(initial):
	case <-ctx.Done():
		return scanossapi.ScanEnvelope{}, ctx.Err()
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		e, err := s.Status(ctx, scanID)
		if err != nil {
			return scanossapi.ScanEnvelope{}, err
		}
		if r != nil {
			r.Scanning(e)
		}

		switch e.Status {
		case scanStateCompleted:
			return e, nil
		case scanStateFailed:
			return scanossapi.ScanEnvelope{}, fmt.Errorf("scan %s failed (phase %q)", e.ScanId, e.Phase)
		case scanStateExpired:
			// Terminal, and unlike failed the id cannot be retried: the session is gone. Without
			// this case an expired session falls through to the poll below and is polled until the
			// caller's context is cancelled — which for the CLI means hanging until Ctrl-C.
			return scanossapi.ScanEnvelope{}, fmt.Errorf("scan %s expired before it completed", e.ScanId)
		case scanStateQueued, scanStateUploading, scanStateScanning:
			// Still moving; wait for the next poll.
		default:
			// A state this client predates. Wait too rather than error: the alternative breaks
			// every existing client the moment the server names a new state.
			s.c.log.Debug("scan reported an unrecognised state", "scanID", e.ScanId, "status", e.Status)
		}

		select {
		case <-ctx.Done():
			return scanossapi.ScanEnvelope{}, ctx.Err()
		case <-ticker.C:
		}
	}
}
