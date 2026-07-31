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

package scanner

import (
	"log/slog"
	"os"
	"sync"

	"github.com/scanoss/scanoss.go/pkg/filter"
	fingerprint "github.com/scanoss/scanoss.go/pkg/fingerprint/wfp"
)

// WorkerPool manages a pool of workers to process files in parallel
type WorkerPool struct {
	numWorkers int
	jobs       chan string
	results    chan *fingerprint.FileFingerprint
	errors     chan error
	wg         sync.WaitGroup
	root       string // when set, WFP "file=" labels are made relative to it
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(numWorkers int) *WorkerPool {
	return &WorkerPool{
		numWorkers: numWorkers,
		jobs:       make(chan string, numWorkers*2),
		results:    make(chan *fingerprint.FileFingerprint, numWorkers*2),
		errors:     make(chan error, numWorkers*2),
	}
}

// Start starts the workers
func (wp *WorkerPool) Start() {
	for i := 0; i < wp.numWorkers; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}
}

// worker processes files from the jobs queue
func (wp *WorkerPool) worker(id int) {
	defer wp.wg.Done()

	for filePath := range wp.jobs {
		// Check if file exists and get info
		stat, err := os.Stat(filePath)
		if err != nil {
			wp.errors <- err
			continue
		}

		// Skip directories
		if stat.IsDir() {
			continue
		}

		fp, err := fingerprint.GenerateFingerprint(filePath, wp.root)
		if err != nil {
			wp.errors <- err
			continue
		}

		wp.results <- fp
	}
}

// Submit sends a file to be processed
func (wp *WorkerPool) Submit(filePath string) {
	wp.jobs <- filePath
}

// Close closes the channels and waits for workers to finish
func (wp *WorkerPool) Close() {
	close(wp.jobs)
	wp.wg.Wait()
	close(wp.results)
	close(wp.errors)
}

// Results returns the results channel
func (wp *WorkerPool) Results() <-chan *fingerprint.FileFingerprint {
	return wp.results
}

// Errors returns the errors channel
func (wp *WorkerPool) Errors() <-chan error {
	return wp.errors
}

// CollectFiles recursively collects the files to scan under rootPath, applying
// the built-in default filters (skip lists, sizes) plus any .gitignore in the
// tree. Kept for backward compatibility; use CollectFilesWithOptions to also
// apply scanoss.json rules or to override the defaults.
func CollectFiles(rootPath string) ([]string, error) {
	res, err := filter.Collect(rootPath, filter.DefaultOptions())
	if err != nil {
		return nil, err
	}
	return res.Files, nil
}

// CollectFilesWithOptions recursively collects the files to scan under rootPath,
// applying the given filter options (defaults, scanoss.json skip rules,
// .gitignore, size bounds). It returns the files to scan and a count of skipped
// files.
func CollectFilesWithOptions(rootPath string, o filter.Options) (*filter.CollectResult, error) {
	return filter.Collect(rootPath, o)
}

// GenerateWFP fingerprints the given files with a worker pool and returns the
// combined WFP byte stream. onProgress, if non-nil, is called as each file
// completes (done, total). Files that fail to fingerprint are skipped and
// returned in the error slice (best-effort).
func GenerateWFP(files []string, workers int, root string, onProgress func(done, total int)) ([]byte, []error) {
	if workers < 1 {
		workers = 1
	}
	slog.Debug("fingerprinting files", "count", len(files), "workers", workers)
	pool := NewWorkerPool(workers)
	pool.root = root // WFP "file=" labels relative to root; empty = absolute
	pool.Start()

	go func() {
		for _, f := range files {
			pool.Submit(f)
		}
		pool.Close()
	}()

	var errs []error
	var errWg sync.WaitGroup
	errWg.Add(1)
	go func() {
		defer errWg.Done()
		for err := range pool.Errors() {
			errs = append(errs, err)
		}
	}()

	var fps []*fingerprint.FileFingerprint
	for fp := range pool.Results() {
		fps = append(fps, fp)
		if onProgress != nil {
			onProgress(len(fps), len(files))
		}
	}
	errWg.Wait()

	return []byte(fingerprint.CombineFingerprints(fps)), errs
}
