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

package cliconfig

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/scanoss/scanoss.go/internal/config"
)

// Recognized setting keys. Keys are snake_case in the file; a new one is an entry
// in registry below plus, if it should affect behaviour, a rung in the resolver.
const (
	KeyAPIURL = "api_url"
	KeyAPIKey = "api_key"
)

// keySpec describes one recognized key.
type keySpec struct {
	// cli is how the key is spelled on the command line: both the flag (--api-key)
	// and the config subcommand argument (config set api-key). They are deliberately
	// the same string — one vocabulary for the user, snake_case only inside the file.
	//
	// Stated per key rather than derived by swapping - for _, so a future key whose
	// command-line name is not a mechanical transform of its stored name is
	// expressible, and so an unrecognized key is never silently rewritten into
	// something that looks recognized.
	cli string
	// def is the built-in default, used when no flag, environment variable, or
	// stored value supplies one. It belongs to the setting rather than to any
	// command's flag, so resolution reports the same default with or without flags —
	// which is what lets `config list` explain a value it has no flag for.
	def string
	// secret marks a key whose value must never be displayed or logged.
	secret bool
}

// registry is the single source of truth for what keys exist. Validation, the
// error message listing valid keys, masking, resolution, and `config list` all
// derive from it, so adding a key does not mean touching each of those in turn.
var registry = map[string]keySpec{
	KeyAPIURL: {cli: "api-url", def: config.DefaultAPIURL},
	KeyAPIKey: {cli: "api-key", secret: true}, // no default: an absent key is absent
}

// storedKeys maps a command-line key to the form stored in the file, built from the
// registry so the two directions cannot disagree. Only the command-line spelling is
// a key here: the CLI has exactly one vocabulary, and `config set api_key` is a
// mistake worth reporting rather than quietly accepting.
var storedKeys = func() map[string]string {
	m := make(map[string]string, len(registry))
	for stored, spec := range registry {
		m[spec.cli] = stored
	}
	return m
}()

// defaultOf returns the built-in default for key, empty when it has none.
func defaultOf(key string) string {
	return registry[key].def
}

// recognizedKeys returns the recognized keys in their stored (snake_case) form,
// sorted. Use CLIKeys for anything a user reads.
func recognizedKeys() []string {
	return slices.Sorted(maps.Keys(registry))
}

// cliKeys returns the recognized keys in the form users type, sorted — the list for
// help and error text.
func cliKeys() []string {
	keys := make([]string, 0, len(registry))
	for _, spec := range registry {
		keys = append(keys, spec.cli)
	}
	sort.Strings(keys)
	return keys
}

// StoredKey maps a command-line key to the form stored in the file: api-key →
// api_key. There is exactly one accepted spelling per setting — the dashed one, as
// listed by CLIKeys — so api_key, API-KEY and apikey are all reported as
// unrecognized. One vocabulary means one thing to document and one error to explain.
//
// Surrounding whitespace is trimmed: that is not a spelling, and leaving it in
// produces a baffling `unrecognized key " api-key"`.
func StoredKey(input string) (string, bool) {
	stored, ok := storedKeys[strings.TrimSpace(input)]
	return stored, ok
}

// CLIKey returns how key is spelled on the command line: api_key → api-key. Every
// recognized key the CLI prints goes through here, so its output can be pasted back
// into a command. It is also the flag name, deliberately the same string.
func CLIKey(key string) string {
	return registry[key].cli
}

// IsRecognized reports whether key — in its stored form — is one this version knows
// about. Keys the CLI does not recognize may still exist in the file: they are
// preserved, not rejected, but nothing reads them.
//
// This takes the stored spelling because it guards the Go API (Set, Unset). Command
// arguments arrive in the command-line spelling and go through StoredKey first.
func IsRecognized(key string) bool {
	_, ok := registry[key]
	return ok
}

// IsSecret reports whether key's value must never be displayed or logged.
func IsSecret(key string) bool {
	return registry[key].secret
}

// EnvName returns the environment variable that overrides key, e.g. api_url →
// SCANOSS_API_URL. It mirrors what viper's SetEnvPrefix("SCANOSS") + AutomaticEnv
// derive, so the resolver and viper cannot disagree about a name.
func EnvName(key string) string {
	return "SCANOSS_" + strings.ToUpper(key)
}

// UnknownKeyError reports a key the CLI does not recognize.
type UnknownKeyError struct {
	Key string
}

func (e *UnknownKeyError) Error() string {
	return fmt.Sprintf("unrecognized key %q; recognized keys are: %s",
		e.Key, strings.Join(cliKeys(), ", "))
}
