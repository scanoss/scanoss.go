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
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

// The sink discards until an application asks for logs: nothing is enabled,
// and calls complete without a destination.
func TestSinkSilentUntilSet(t *testing.T) {
	Set(nil)
	t.Cleanup(func() { Set(nil) })

	for _, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError} {
		if Enabled(level) {
			t.Errorf("level %v enabled before Set — the sink must discard", level)
		}
	}
	Debug("into the void", "k", "v")
	Warn("into the void")
}

// Set(lg) routes the module's records to lg — the guarantee behind scanoss.SetLogger.
func TestSetRoutesRecords(t *testing.T) {
	t.Cleanup(func() { Set(nil) })

	var buf bytes.Buffer
	Set(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	Debug("debug msg", "path", "a.go")
	Warn("warn msg", "count", 3)

	out := buf.String()
	for _, want := range []string{"debug msg", "path=a.go", "warn msg", "count=3"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q; got %q", want, out)
		}
	}

	// Set(nil) restores silence: nothing further reaches the old logger.
	Set(nil)
	before := buf.Len()
	Debug("after reset")
	Warn("after reset")
	if buf.Len() != before {
		t.Errorf("records still arrive after Set(nil): %q", buf.String()[before:])
	}
}

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
