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
	"fmt"
	"os"
)

// Status-line UI shared by every command. Human-facing progress notices on stderr are prefixed
// with an icon and, when stderr is an interactive terminal, colored: warnings yellow, info dim,
// success green. When stderr is not a terminal (piped or redirected) the line degrades to plain
// text with no icon or color, so logs and `--output` pipelines stay clean.
const (
	iconWarn = "⚠"
	iconInfo = "ℹ"
	iconOK   = "✓"

	ansiReset = "\x1b[0m"
	// Warning/success are 24-bit truecolor escapes (the One Dark palette) so each hue is fixed
	// rather than depending on the terminal theme (the bright indices read harsher); terminals
	// without truecolor approximate them to the nearest palette color. Info is faint rather than
	// colored — a palette-independent dim attribute that recedes as secondary chatter.
	ansiYellow = "\x1b[38;2;229;192;123m" // #e5c07b
	ansiGreen  = "\x1b[38;2;152;195;121m" // #98c379
	ansiDim    = "\x1b[2m"
)

// stderrIsTerminal reports whether stderr is an interactive character device. Uses only the
// standard library (os.ModeCharDevice), which is set for consoles on Windows and character devices
// on macOS/Linux — no isatty/x-term dependency.
func stderrIsTerminal() bool {
	fi, err := os.Stderr.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

var (
	// noticeDecorated: add an icon (only when writing to a terminal — plain text when piped).
	noticeDecorated = stderrIsTerminal()
	// noticeColored: additionally emit ANSI color. Honors NO_COLOR, and on Windows requires the
	// console's virtual-terminal mode to be enabled (enableVirtualTerminal); a no-op elsewhere.
	noticeColored = noticeDecorated && os.Getenv("NO_COLOR") == "" && enableVirtualTerminal()
)

// noticeLine renders msg as a status line: "<icon> msg" on a terminal (colored when enabled), or
// bare msg when stderr is not a terminal. It returns the string without a trailing newline so it
// can also be routed through the progress container (which writes lines above the bars).
func noticeLine(icon, color, msg string) string {
	if !noticeDecorated {
		return msg
	}
	line := icon + " " + msg
	if noticeColored {
		return color + line + ansiReset
	}
	return line
}

func warnLine(format string, a ...any) string {
	return noticeLine(iconWarn, ansiYellow, fmt.Sprintf(format, a...))
}

func infoLine(format string, a ...any) string {
	return noticeLine(iconInfo, ansiDim, fmt.Sprintf(format, a...))
}

func okLine(format string, a ...any) string {
	return noticeLine(iconOK, ansiGreen, fmt.Sprintf(format, a...))
}

// warnf / infof / okf print a decorated status line to stderr.
func warnf(format string, a ...any) { fmt.Fprintln(os.Stderr, warnLine(format, a...)) }
func infof(format string, a ...any) { fmt.Fprintln(os.Stderr, infoLine(format, a...)) }
func okf(format string, a ...any)   { fmt.Fprintln(os.Stderr, okLine(format, a...)) }
