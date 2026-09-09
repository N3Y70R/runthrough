// SPDX-License-Identifier: GPL-3.0-or-later

// Package artifact reads the contract a driver artifact declares about
// itself.
//
// This is discovery without guesswork: every ${VAR} the file interpolates
// without a default MUST be defined or the runtime refuses to start, and
// every env file it references must exist. Nobody has to declare these in the
// catalog — the artifact already did, and the tool can simply read it.
//
// Anything softer than this (a repository's .env.example, a framework's
// configuration calls) belongs in a suggestion, not in a check: a variable
// that appears in an example file may well have a default in code, and a
// check that cries wolf teaches people to ignore checks.
package artifact

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Variable is one interpolation the artifact performs.
type Variable struct {
	Name string `json:"name"`
	// HasDefault marks ${VAR:-something}: absent is fine, the artifact says
	// what to use instead.
	HasDefault bool `json:"has_default"`
	// Explicit marks ${VAR:?message}: the artifact itself declares it
	// mandatory and carries the message to show.
	Explicit bool   `json:"explicit,omitempty"`
	Message  string `json:"message,omitempty"`
}

// EnvFileRef is one env file the artifact loads.
type EnvFileRef struct {
	Service  string `json:"service"`
	Path     string `json:"path"`
	Required bool   `json:"required"`
}

// Contract is what the artifact needs in order to run at all.
type Contract struct {
	Path      string       `json:"path"`
	Variables []Variable   `json:"variables"`
	EnvFiles  []EnvFileRef `json:"env_files"`
}

// ${NAME}, ${NAME:-default}, ${NAME-default}, ${NAME:?message}, ${NAME?message}
var braced = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)((:?[-?])([^}]*))?\}`)

// $NAME, without braces.
var bare = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)

// Read parses an artifact and returns what it requires.
func Read(path string) (*Contract, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := &Contract{Path: path}
	c.Variables = variables(string(data))

	files, err := envFiles(data)
	if err != nil {
		// A file we cannot parse still yields its variables, which is the
		// more valuable half; the caller decides what to do with the error.
		return c, err
	}
	c.EnvFiles = files
	return c, nil
}

func variables(text string) []Variable {
	// $$ is an escaped dollar sign, not an interpolation.
	text = strings.ReplaceAll(text, "$$", "")

	found := map[string]Variable{}
	record := func(v Variable) {
		// A variable used twice is required if any use requires it.
		if old, ok := found[v.Name]; ok {
			v.HasDefault = old.HasDefault || v.HasDefault
			if old.Explicit {
				v.Explicit, v.Message = true, old.Message
			}
		}
		found[v.Name] = v
	}

	for _, m := range braced.FindAllStringSubmatch(text, -1) {
		v := Variable{Name: m[1]}
		switch {
		case strings.HasSuffix(m[3], "-"):
			v.HasDefault = true
		case strings.HasSuffix(m[3], "?"):
			v.Explicit = true
			v.Message = strings.TrimSpace(m[4])
		}
		record(v)
	}
	withoutBraced := braced.ReplaceAllString(text, "")
	for _, m := range bare.FindAllStringSubmatch(withoutBraced, -1) {
		record(Variable{Name: m[1]})
	}

	out := make([]Variable, 0, len(found))
	for _, v := range found {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// envFiles extracts the env_file declarations, in every shape Compose
// accepts: a string, a list of strings, or a list of {path, required}.
func envFiles(data []byte) ([]EnvFileRef, error) {
	var doc struct {
		Services map[string]struct {
			EnvFile yaml.Node `yaml:"env_file"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("reading the artifact: %w", err)
	}

	var out []EnvFileRef
	names := make([]string, 0, len(doc.Services))
	for name := range doc.Services {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		node := doc.Services[name].EnvFile
		out = append(out, refsFrom(name, &node)...)
	}
	return out, nil
}

func refsFrom(service string, node *yaml.Node) []EnvFileRef {
	if node == nil || node.Kind == 0 {
		return nil
	}
	switch node.Kind {
	case yaml.ScalarNode:
		return []EnvFileRef{{Service: service, Path: node.Value, Required: true}}
	case yaml.SequenceNode:
		var out []EnvFileRef
		for _, item := range node.Content {
			switch item.Kind {
			case yaml.ScalarNode:
				out = append(out, EnvFileRef{Service: service, Path: item.Value, Required: true})
			case yaml.MappingNode:
				ref := EnvFileRef{Service: service, Required: true}
				for i := 0; i+1 < len(item.Content); i += 2 {
					key, value := item.Content[i].Value, item.Content[i+1]
					switch key {
					case "path":
						ref.Path = value.Value
					case "required":
						ref.Required = value.Value != "false"
					}
				}
				out = append(out, ref)
			}
		}
		return out
	}
	return nil
}

// Missing returns the variables that must be defined and are not. lookup
// reports whether a name resolves, without exposing what it resolves to.
func (c *Contract) Missing(lookup func(string) bool) []Variable {
	var out []Variable
	for _, v := range c.Variables {
		if v.HasDefault || lookup(v.Name) {
			continue
		}
		out = append(out, v)
	}
	return out
}

// Interpolate replaces ${VAR} and $VAR using resolved values, leaving
// unknown names untouched so that a caller can tell them apart.
func Interpolate(s string, values map[string]string) string {
	replace := func(name string) (string, bool) {
		v, ok := values[name]
		if !ok {
			v, ok = os.LookupEnv(name)
		}
		return v, ok
	}
	s = braced.ReplaceAllStringFunc(s, func(match string) string {
		m := braced.FindStringSubmatch(match)
		if v, ok := replace(m[1]); ok {
			return v
		}
		if strings.HasSuffix(m[3], "-") {
			return m[4]
		}
		return match
	})
	return bare.ReplaceAllStringFunc(s, func(match string) string {
		if v, ok := replace(strings.TrimPrefix(match, "$")); ok {
			return v
		}
		return match
	})
}
