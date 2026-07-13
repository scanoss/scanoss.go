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

package batch

import (
	"strings"
	"sync"

	"github.com/scanoss/scanoss.go/internal/models"
)

// Batcher groups fingerprints into batches according to a maximum size
type Batcher struct {
	maxSize      int
	currentBatch []*models.FileFingerprint
	currentSize  int
	batches      [][]*models.FileFingerprint
}

// NewBatcher creates a new fingerprint batcher
func NewBatcher(maxSize int) *Batcher {
	return &Batcher{
		maxSize:      maxSize,
		currentBatch: make([]*models.FileFingerprint, 0),
		currentSize:  0,
		batches:      make([][]*models.FileFingerprint, 0),
	}
}

// Add adds a fingerprint to the batcher
// If it exceeds the maximum size, creates a new batch
func (b *Batcher) Add(fp *models.FileFingerprint) {
	fpSize := len(fp.Fingerprint)

	// If adding this fingerprint exceeds the limit, first save the current batch
	if b.currentSize+fpSize > b.maxSize && len(b.currentBatch) > 0 {
		b.flush()
	}

	// Add to current batch
	b.currentBatch = append(b.currentBatch, fp)
	b.currentSize += fpSize
}

// flush saves the current batch to the list of batches
func (b *Batcher) flush() {
	if len(b.currentBatch) > 0 {
		b.batches = append(b.batches, b.currentBatch)
		b.currentBatch = make([]*models.FileFingerprint, 0)
		b.currentSize = 0
	}
}

// GetBatches returns all batches, including the current batch if not empty
func (b *Batcher) GetBatches() [][]*models.FileFingerprint {
	// Ensure the last batch is saved
	b.flush()
	return b.batches
}

// BatchWithMetadata wraps a batch with metadata about its position
type BatchWithMetadata struct {
	Data         []*models.FileFingerprint
	IsFinalChunk bool
}

// StreamingBatcher groups fingerprints and sends them to a channel when they reach the maximum size
type StreamingBatcher struct {
	maxSize      int
	currentBatch []*models.FileFingerprint
	currentSize  int
	output       chan BatchWithMetadata
	mu           sync.Mutex
}

// NewStreamingBatcher creates a new batcher in streaming mode
func NewStreamingBatcher(maxSize int) *StreamingBatcher {
	return &StreamingBatcher{
		maxSize:      maxSize,
		currentBatch: make([]*models.FileFingerprint, 0),
		currentSize:  0,
		output:       make(chan BatchWithMetadata, 10),
	}
}

// Add adds a fingerprint and sends the batch if it reaches the limit
func (sb *StreamingBatcher) Add(fp *models.FileFingerprint) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	fpSize := len(fp.Fingerprint)

	// If adding this fingerprint exceeds the limit, we send the current batch
	if sb.currentSize+fpSize > sb.maxSize && len(sb.currentBatch) > 0 {
		sb.flushLocked()
	}

	// Add to current batch
	sb.currentBatch = append(sb.currentBatch, fp)
	sb.currentSize += fpSize
}

// flushLocked sends the current batch to the channel (must be called with lock)
func (sb *StreamingBatcher) flushLocked() {
	if len(sb.currentBatch) > 0 {
		sb.output <- BatchWithMetadata{
			Data:         sb.currentBatch,
			IsFinalChunk: false, // Will be updated on Close() for the final batch
		}
		sb.currentBatch = make([]*models.FileFingerprint, 0)
		sb.currentSize = 0
	}
}

// Close closes the batcher sending the last pending batch marked as final
func (sb *StreamingBatcher) Close() {
	sb.mu.Lock()
	// Send final batch with IsFinalChunk=true
	if len(sb.currentBatch) > 0 {
		sb.output <- BatchWithMetadata{
			Data:         sb.currentBatch,
			IsFinalChunk: true,
		}
		sb.currentBatch = make([]*models.FileFingerprint, 0)
		sb.currentSize = 0
	}
	sb.mu.Unlock()
	close(sb.output)
}

// Batches returns the batches channel
func (sb *StreamingBatcher) Batches() <-chan BatchWithMetadata {
	return sb.output
}

// CombineFingerprints combines multiple fingerprints into a single WFP string
// Ensures there is a blank line between each file.
//
// Uses a strings.Builder with a pre-sized buffer: naive `result += ...` in a
// loop is O(n²) (each += copies the whole accumulated string), which dominated
// wall-clock time for large scans — e.g. ~38s to assemble a 16 MB WFP from
// ~8900 files. Builder grows amortized O(1), making this linear.
func CombineFingerprints(fps []*models.FileFingerprint) string {
	var total int
	for _, fp := range fps {
		// fingerprint + up to one missing trailing newline + one blank line
		total += len(fp.Fingerprint) + 2
	}

	var b strings.Builder
	b.Grow(total)
	for _, fp := range fps {
		b.WriteString(fp.Fingerprint)
		// Ensure each fingerprint ends with newline
		if len(fp.Fingerprint) > 0 && fp.Fingerprint[len(fp.Fingerprint)-1] != '\n' {
			b.WriteByte('\n')
		}
		// Add blank line after each file (required by WFP format)
		b.WriteByte('\n')
	}
	return b.String()
}
