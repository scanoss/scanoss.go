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

package logging

import (
	"context"
	"log/slog"
	"testing"
)

func TestConfigureLevel(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name         string
		verbose      bool
		debugEnabled bool
		infoEnabled  bool
	}{
		{"default is warn (debug+info off)", false, false, false},
		{"verbose is debug (debug+info on)", true, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger := Configure(tc.verbose)
			if got := logger.Enabled(ctx, slog.LevelDebug); got != tc.debugEnabled {
				t.Errorf("Debug enabled = %v, want %v", got, tc.debugEnabled)
			}
			if got := logger.Enabled(ctx, slog.LevelInfo); got != tc.infoEnabled {
				t.Errorf("Info enabled = %v, want %v", got, tc.infoEnabled)
			}
			// Warn and Error are always enabled.
			if !logger.Enabled(ctx, slog.LevelWarn) {
				t.Error("Warn should always be enabled")
			}
			// Configure installs the default logger.
			if slog.Default() != logger {
				t.Error("Configure did not install the returned logger as the default")
			}
		})
	}
}
