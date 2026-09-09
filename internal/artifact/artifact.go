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

// Service is what the artifact says about one service. It is the authority
// on this: the catalog describes intent, the artifact describes what will
// actually run.
type Service struct {
	Name string `json:"name"`
	// Builds is true when the service has a build section. A service that
	// only pulls an image cannot be rebuilt, whatever the catalog says.
	Builds bool `json:"builds"`
	// Healthcheck is true when the artifact defines one, which is the only
	// trustworthy signal that a service is actually up.
	Healthcheck bool `json:"healthcheck"`
	// Mounts are the host paths bind-mounted into the service.
	Mounts []string `json:"mounts,omitempty"`
}

// Contract is what the artifact needs in order to run at all.
type Contract struct {
	Path      string             `json:"path"`
	Variables []Variable         `json:"variables"`
	EnvFiles  []EnvFileRef       `json:"env_files"`
	Services  map[string]Service `json:"services,omitempty"`
}

// Service returns what the artifact says about a service.
func (c *Contract) Service(name string) (Service, bool) {
	s, ok := c.Services[name]
	return s, ok
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

	files, services, err := structure(data)
	if err != nil {
		// A file we cannot parse still yields its variables, which is the
		// more valuable half; the caller decides what to do with the error.
		return c, err
	}
	c.EnvFiles = files
	c.Services = services
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

// structure extracts what the artifact declares per service: env files, a
// build section, a healthcheck, and bind-mounted host paths.
func structure(data []byte) ([]EnvFileRef, map[string]Service, error) {
	var doc struct {
		Services map[string]struct {
			EnvFile     yaml.Node `yaml:"env_file"`
			Build       yaml.Node `yaml:"build"`
			Healthcheck yaml.Node `yaml:"healthcheck"`
			Volumes     []string  `yaml:"volumes"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, nil, fmt.Errorf("reading the artifact: %w", err)
	}

	var refs []EnvFileRef
	services := map[string]Service{}
	names := make([]string, 0, len(doc.Services))
	for name := range doc.Services {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		node := doc.Services[name]
		refs = append(refs, refsFrom(name, &node.EnvFile)...)
		services[name] = Service{
			Name:        name,
			Builds:      node.Build.Kind != 0,
			Healthcheck: node.Healthcheck.Kind != 0,
			Mounts:      hostMounts(node.Volumes),
		}
	}
	return refs, services, nil
}

// hostMounts keeps the bind mounts — the ones whose source is a path — and
// drops named volumes, which Docker creates on demand.
func hostMounts(volumes []string) []string {
	var out []string
	for _, v := range volumes {
		source, _, ok := strings.Cut(v, ":")
		if !ok || source == "" {
			continue
		}
		if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") ||
			strings.HasPrefix(source, "/") || strings.HasPrefix(source, "~") ||
			strings.HasPrefix(source, "$") {
			out = append(out, source)
		}
	}
	return out
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
