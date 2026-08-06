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

package wfp

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/scanoss/scanoss.go/internal/logging"
	"github.com/scanoss/scanoss.go/pkg/filter"
)

// workerPool manages a pool of workers to process files in parallel.
//
// It is unexported because using it correctly takes three concurrent participants — one
// submitting, one draining errors, one draining results — and the channels are buffered
// at numWorkers*2, so anything that submits more files than that without draining
// deadlocks. Generate is that dance, done once and tested; publishing the pool would
// publish a mandatory protocol whose only failure mode is a hang.
type workerPool struct {
	numWorkers int
	jobs       chan string
	results    chan *FileFingerprint
	errors     chan error
	wg         sync.WaitGroup
	root       string // when set, WFP "file=" labels are made relative to it
}

// newWorkerPool creates a new worker pool
func newWorkerPool(numWorkers int) *workerPool {
	return &workerPool{
		numWorkers: numWorkers,
		jobs:       make(chan string, numWorkers*2),
		results:    make(chan *FileFingerprint, numWorkers*2),
		errors:     make(chan error, numWorkers*2),
	}
}

// start starts the workers
func (wp *workerPool) start() {
	for i := 0; i < wp.numWorkers; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}
}

// worker processes files from the jobs queue
func (wp *workerPool) worker(id int) {
	defer wp.wg.Done()

	for filePath := range wp.jobs {
		// Check if file exists and get info
		stat, err := os.Stat(filePath)
		if err != nil {
			wp.errors <- err
			continue
		}

		// A directory cannot be fingerprinted; reporting it beats returning an
		// empty result to whoever put it in the list.
		if stat.IsDir() {
			wp.errors <- fmt.Errorf("%s: is a directory, not fingerprinted", filePath)
			continue
		}

		fp, err := generateFingerprint(filePath, wp.root)
		if err != nil {
			wp.errors <- err
			continue
		}

		wp.results <- fp
	}
}

// submit sends a file to be processed
func (wp *workerPool) submit(filePath string) {
	wp.jobs <- filePath
}

// close closes the channels and waits for workers to finish
func (wp *workerPool) close() {
	close(wp.jobs)
	wp.wg.Wait()
	close(wp.results)
	close(wp.errors)
}

// Result is what Generate produced: the stream to upload, the per-file detail behind it,
// and the files it could not fingerprint.
//
// WFP and Files are two views of one run, not a choice: a scan uploads WFP, while a caller
// reporting on individual files needs Hash and Size, which the combined stream does not
// carry. Errors is a field rather than a second return value because fingerprinting is
// best-effort — a run that skipped three unreadable files still produced a usable WFP —
// and a caller that ignores it should have to say so.
type Result struct {
	WFP     []byte            // the combined stream, ready to upload
	Files   []FileFingerprint // per-file detail, sorted by path
	Errors  []error           // files that could not be fingerprinted
	Skipped int               // files the filters excluded; always 0 from Files, which selects nothing
}

// Folder collects the files worth fingerprinting under dir and fingerprints them. A nil filters
// uses the fingerprinting profile, which is what a caller holding a directory and no opinion
// about the rules wants.
//
// It is the entry point to reach for with a directory in hand: Files fingerprints whatever it is
// given, so building the list yourself means deciding for yourself what a scan should skip.
func Folder(dir string, filters *filter.Options, workers int, onProgress func(done, total int)) (Result, error) {
	o := filter.Fingerprinting(nil)
	if filters != nil {
		o = *filters
	}

	collected, err := filter.Collect(dir, o)
	if err != nil {
		return Result{}, fmt.Errorf("collecting files: %w", err)
	}

	// The WFP labels paths relative to the directory walked. A file handed to Folder is its
	// own collection of one, and the label belongs to the directory holding it.
	root := dir
	if info, statErr := os.Stat(dir); statErr == nil && !info.IsDir() {
		root = filepath.Dir(dir)
	}
	if abs, absErr := filepath.Abs(root); absErr == nil {
		root = abs
	}

	res := Files(collected.Files, workers, root, onProgress)
	res.Skipped = collected.SkippedCount
	return res, nil
}

// Files fingerprints the given files through a bounded worker pool. onProgress, if non-nil, is
// called as each file completes (done, total), counting failures too, so done reaches total.
//
// It does no selection: which files are worth fingerprinting was decided by whoever built the
// list. Fingerprinting one file is Files with a one-file slice — a separate single-file function
// would be a second way to do the same thing, and the caller would still have to combine the
// output itself to upload it.
//
// TODO(#77): filter here as well, once the rules can be applied to a list. Folder filters and
// Files does not, which their names do not suggest: a caller that hands over every file in a
// directory gets a WFP full of files a scan would have skipped.
func Files(files []string, workers int, root string, onProgress func(done, total int)) Result {
	if workers < 1 {
		workers = 1
	}
	logging.Debug("fingerprinting files", "count", len(files), "workers", workers)
	pool := newWorkerPool(workers)
	pool.root = root // WFP "file=" labels relative to root; empty = absolute
	pool.start()

	go func() {
		for _, f := range files {
			pool.submit(f)
		}
		pool.close()
	}()

	// One loop over both channels so a failed file advances the progress too:
	// a run with failures still ends at done == total.
	var (
		errs []error
		fps  []*FileFingerprint
		done int
	)
	results, failures := pool.results, pool.errors
	for results != nil || failures != nil {
		select {
		case fp, ok := <-results:
			if !ok {
				results = nil
				continue
			}
			fps = append(fps, fp)
		case err, ok := <-failures:
			if !ok {
				failures = nil
				continue
			}
			errs = append(errs, err)
		}
		done++
		if onProgress != nil {
			onProgress(done, len(files))
		}
	}

	// Completion order depends on worker scheduling; sorting makes the WFP
	// byte-reproducible across runs, whatever the thread count.
	slices.SortFunc(fps, func(a, b *FileFingerprint) int {
		return strings.Compare(a.Path, b.Path)
	})

	out := make([]FileFingerprint, 0, len(fps))
	for _, fp := range fps {
		out = append(out, *fp)
	}
	return Result{WFP: []byte(combineFingerprints(fps)), Files: out, Errors: errs}
}
