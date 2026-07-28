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

// Package cliconfig stores and resolves the CLI's own settings — the API endpoint
// and credentials the user keeps in $HOME/.scanoss/settings.json.
//
// This is NOT a project's scanoss.json (see pkg/settings), which holds that
// project's BOM and skip rules. The two are unrelated despite the similar file
// name.
//
// The package is deliberately under internal/: reading ambient state (environment
// variables, a file in $HOME) is correct for a CLI and wrong for a library, so the
// SDK in pkg/ cannot import it even by accident.
package cliconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/viper"
)

// Recognized setting keys. Keys are snake_case in the file; a new one is an entry
// in registry below plus, if it should affect behaviour, a rung in the resolver.
const (
	KeyAPIURL = "api_url"
	KeyAPIKey = "api_key"
)

const (
	dirName  = ".scanoss"
	fileName = "settings.json"

	// dirPerm and filePerm are applied only to paths this package creates: the
	// file holds an API key, so it must not be group- or world-readable.
	dirPerm  fs.FileMode = 0o700
	filePerm fs.FileMode = 0o600
)

// keySpec describes one recognized key.
type keySpec struct {
	// flag is the CLI flag that overrides this key. Stated rather than derived from
	// the key name, so the coupling between the two spellings is visible here.
	flag string
	// secret marks a key whose value must never be displayed or logged.
	secret bool
}

// registry is the single source of truth for what keys exist. Validation, the
// error message listing valid keys, masking, resolution, and `config list` all
// derive from it, so adding a key does not mean touching each of those in turn.
var registry = map[string]keySpec{
	KeyAPIURL: {flag: "api-url"},
	KeyAPIKey: {flag: "api-key", secret: true},
}

// homeDir is indirected so tests can point the package at a temporary home
// without depending on HOME/USERPROFILE semantics per platform.
var homeDir = os.UserHomeDir

// RecognizedKeys returns the recognized keys, sorted, for help and error text.
func RecognizedKeys() []string {
	return slices.Sorted(maps.Keys(registry))
}

// IsRecognized reports whether key is one this version knows about. Keys the CLI
// does not recognize may still exist in the file — they are preserved, not
// rejected — but they cannot be set or read through the config command.
func IsRecognized(key string) bool {
	_, ok := registry[key]
	return ok
}

// FlagName returns the CLI flag that overrides key, e.g. api_url → api-url.
// It is empty for a key the CLI does not recognize.
func FlagName(key string) string {
	return registry[key].flag
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
		e.Key, strings.Join(RecognizedKeys(), ", "))
}

// Path returns the absolute path of the settings file, whether or not it exists.
func Path() (string, error) {
	home, err := homeDir()
	if err != nil {
		return "", fmt.Errorf("locating home directory: %w", err)
	}
	return filepath.Join(home, dirName, fileName), nil
}

// Config is the settings as stored on disk, including keys this version does not
// recognize.
type Config struct {
	values map[string]any
}

// Load reads the settings file. A missing file is not an error — it yields an
// empty Config, and nothing is created. A malformed file IS an error: falling back
// to defaults would silently scan against the wrong endpoint.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	values, err := read(path)
	if err != nil {
		return nil, err
	}
	return &Config{values: values}, nil
}

// Get returns the stored value of key, rendered as a string. An absent key and an
// empty value are both reported as unset, so `"api_key": ""` in the file behaves
// like no api_key at all.
func (c *Config) Get(key string) (string, bool) {
	raw, ok := c.values[key]
	if !ok || raw == nil {
		return "", false
	}
	value := fmt.Sprintf("%v", raw)
	if value == "" {
		return "", false
	}
	return value, true
}

// Keys returns every key present in the file, sorted — including unrecognized
// ones, so `config list` can show what a hand-edited file actually holds.
func (c *Config) Keys() []string {
	return slices.Sorted(maps.Keys(c.values))
}

// Set stores value under key, creating ~/.scanoss and the file if missing. Keys
// already in the file that this version does not recognize are left untouched.
func Set(key, value string) error {
	if !IsRecognized(key) {
		return &UnknownKeyError{Key: key}
	}
	return mutate(func(values map[string]any) {
		values[key] = value
	})
}

// Unset removes key from the file. Removing a key that is not there succeeds:
// the requested end state is what the caller asked for.
func Unset(key string) error {
	if !IsRecognized(key) {
		return &UnknownKeyError{Key: key}
	}
	return mutate(func(values map[string]any) {
		delete(values, key)
	})
}

// fileViper returns a viper that has read the settings file and sees nothing else:
// no environment, no flags. A missing file yields an empty instance, since "not
// configured" is a normal state; a malformed one is an error, because falling back
// to defaults would silently scan against the wrong endpoint.
//
// Keeping a file-only instance is what makes a safe write possible: viper's merged
// view must never reach the file (see write).
func fileViper(path string) (*viper.Viper, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("json")

	// With SetConfigFile a missing file surfaces as fs.ErrNotExist, not
	// viper.ConfigFileNotFoundError — that one is only produced by path search.
	if err := v.ReadInConfig(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return v, nil
}

// read parses the settings file into a plain map, which keeps every key found and
// is what preserves unrecognized ones across a write.
func read(path string) (map[string]any, error) {
	v, err := fileViper(path)
	if err != nil {
		return nil, err
	}
	return v.AllSettings(), nil
}

// mutate applies fn to the stored settings and writes the result back.
func mutate(fn func(map[string]any)) error {
	path, err := Path()
	if err != nil {
		return err
	}
	values, err := read(path)
	if err != nil {
		return err
	}
	fn(values)
	return write(path, values)
}

// write serializes values and replaces the settings file atomically.
//
// viper.WriteConfig is never used: it serializes viper's merged view, so a write
// would persist values that came from the environment or a flag — `config set
// api_url X` with SCANOSS_API_KEY exported would put that key on disk. It also
// writes 0644 and truncates in place rather than renaming.
func write(path string, values map[string]any) error {
	// Marshalling a map sorts the keys, so the file stays diff-friendly.
	data, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding settings: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := ensureDir(dir); err != nil {
		return err
	}

	// Write a sibling temp file, then rename: a reader sees either the old file or
	// the new one, never a half-written mix.
	tmp, err := os.CreateTemp(dir, fileName+".tmp*")
	if err != nil {
		return fmt.Errorf("creating temporary file in %s: %w", dir, err)
	}
	// Removing the temp file is a no-op once the rename below succeeds.
	defer func() { _ = os.Remove(tmp.Name()) }()

	if err := fill(tmp, data); err != nil {
		return fmt.Errorf("writing %s: %w", tmp.Name(), err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	return nil
}

// fill tightens permissions on f, writes data, and closes it — reporting the close
// error when nothing earlier failed, since a failed close can mean lost bytes.
func fill(f *os.File, data []byte) (err error) {
	defer func() {
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
	}()
	if err := f.Chmod(filePerm); err != nil {
		return err
	}
	_, err = f.Write(data)
	return err
}

// ensureDir creates the settings directory when missing. A directory that already
// exists is left exactly as it is, including its permissions: tightening a
// directory the user created is not this command's call.
func ensureDir(dir string) error {
	info, err := os.Stat(dir)
	switch {
	case err == nil:
		if !info.IsDir() {
			return fmt.Errorf("%s exists but is not a directory", dir)
		}
		return nil
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("checking %s: %w", dir, err)
	}

	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	// MkdirAll applies the umask, so set the mode explicitly — this directory
	// holds a credential file.
	if err := os.Chmod(dir, dirPerm); err != nil {
		return fmt.Errorf("setting permissions on %s: %w", dir, err)
	}
	return nil
}
