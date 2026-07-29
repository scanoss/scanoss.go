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

package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// createCancellableContext creates a context that can be cancelled with CTRL+C
func createCancellableContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	// Setup signal handling for graceful cancellation
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Fprint(os.Stderr, "\n\n")
		warnf("Received interrupt signal. Cancelling...")
		cancel()
	}()

	return ctx, cancel
}

// validateSizeBounds checks the --min-size / --max-size pair before any file is
// read. On either bound, 0 means "no bound" (no minimum / unlimited), so 0 is
// always valid
func validateSizeBounds(min, max int64) error {
	if min < 0 {
		return fmt.Errorf("--min-size must not be negative (got %d); use 0 for no minimum", min)
	}
	if max < 0 {
		return fmt.Errorf("--max-size must not be negative (got %d); use 0 for unlimited", max)
	}
	if max > 0 && min > max {
		return fmt.Errorf("--min-size (%d) must not exceed --max-size (%d): no file could match", min, max)
	}
	return nil
}
