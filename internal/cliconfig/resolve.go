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
	"log/slog"
	"os"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Source names where a resolved value came from. `config list` reports it, and it
// is the only observable signal about a secret, whose value is never displayed.
type Source string

const (
	SourceFlag    Source = "flag"
	SourceEnv     Source = "env"
	SourceFile    Source = "config file"
	SourceDefault Source = "default"
	SourceUnset   Source = "unset"
)

// Resolved is one setting's effective value and where it came from.
type Resolved struct {
	Value  string
	Source Source
}

// API holds the resolved endpoint settings every API command needs.
type API struct {
	URL string
	Key string
}

// Resolver answers "what value will this command actually use?" for each setting,
// applying flag > environment > config file > flag default.
//
// viper resolves the value. The ladder below is walked a second time to name the
// source, which viper does not expose per key (viper.Debug prints every layer to
// stdout and is not usable programmatically). The duplication is deliberate and
// covered by TestResolverValueAndSourceAgree.
type Resolver struct {
	viper *viper.Viper
	flags *pflag.FlagSet
}

// NewResolver reads the settings file once and binds the flags it recognizes.
// A command that does not declare a given flag is fine: that rung is simply
// skipped, which is what leaves `wfp` and `sbom` unaffected.
func NewResolver(flags *pflag.FlagSet) (*Resolver, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	v, err := fileViper(path)
	if err != nil {
		return nil, err
	}
	// AutomaticEnv derives SCANOSS_API_URL from api_url, so the variable names are
	// not spelled out twice; EnvName mirrors the same rule for reporting.
	v.SetEnvPrefix("SCANOSS")
	v.AutomaticEnv()

	if flags == nil {
		flags = pflag.NewFlagSet("", pflag.ContinueOnError)
	}
	for key, spec := range registry {
		flag := flags.Lookup(spec.cli)
		if flag == nil {
			continue // this command does not offer the flag
		}
		if err := v.BindPFlag(key, flag); err != nil {
			return nil, err
		}
	}
	return &Resolver{viper: v, flags: flags}, nil
}

// Key returns the effective value of one recognized key and its source.
func (r *Resolver) Key(key string) Resolved {
	resolved := r.walk(key)

	// Diagnostics name the setting the way the user types it, like every other
	// message. A secret's value must not reach a log, so only its source is recorded.
	if IsSecret(key) {
		slog.Debug("resolved "+CLIKey(key), "source", resolved.Source)
	} else {
		slog.Debug("resolved "+CLIKey(key), "source", resolved.Source, "value", resolved.Value)
	}
	return resolved
}

// walk is the precedence ladder. It mirrors what viper's own lookup does, purely
// so the winning rung can be named.
func (r *Resolver) walk(key string) Resolved {
	flag := r.flags.Lookup(CLIKey(key))

	// An explicitly-typed flag wins: the value a flag holds cannot tell you whether
	// the user chose it, so Changed is the only reliable signal.
	if flag != nil && flag.Changed {
		return Resolved{Value: r.viper.GetString(key), Source: SourceFlag}
	}
	// An empty environment variable counts as unset, matching viper's own
	// AutomaticEnv behaviour when AllowEmptyEnv is off (the default).
	if os.Getenv(EnvName(key)) != "" {
		return Resolved{Value: r.viper.GetString(key), Source: SourceEnv}
	}
	// InConfig alone is not enough: viper returns a present-but-empty config value
	// ahead of the flag default, and an empty value means unset.
	if r.viper.InConfig(key) && r.viper.GetString(key) != "" {
		return Resolved{Value: r.viper.GetString(key), Source: SourceFile}
	}
	// The built-in default is the last rung, read from the registry rather than from
	// the flag: `config list` declares no API flags and must still be able to explain
	// where a value came from. An absent default is not a value — report unset.
	if def := Default(key); def != "" {
		return Resolved{Value: def, Source: SourceDefault}
	}
	return Resolved{Source: SourceUnset}
}

// API returns the resolved endpoint settings.
func (r *Resolver) API() API {
	return API{
		URL: r.Key(KeyAPIURL).Value,
		Key: r.Key(KeyAPIKey).Value,
	}
}

// ResolveAPI returns the API settings a command should use, applying
// flag > environment > config file > flag default. This is the entry point for
// commands; use a Resolver directly when the sources matter too.
func ResolveAPI(flags *pflag.FlagSet) (API, error) {
	r, err := NewResolver(flags)
	if err != nil {
		return API{}, err
	}
	return r.API(), nil
}
