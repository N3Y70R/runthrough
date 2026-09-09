// SPDX-License-Identifier: GPL-3.0-or-later

// Package catalog is the declarative model of what a stack is made of.
//
// The catalog is the single source of truth for services, ports, dependencies
// and infrastructure (T-05). It is deliberately runtime-neutral: no term here
// belongs to Compose, Podman or Kubernetes, so a driver can translate it
// without the domain knowing which one is in use (R-02).
package catalog

// Catalog is a whole manifest, including anything it pulled in via include.
type Catalog struct {
	Version    int                   `yaml:"version"`
	Include    []string              `yaml:"include"`
	Defaults   Defaults              `yaml:"defaults"`
	Ecosystems map[string]*Ecosystem `yaml:"ecosystems"`
	Infra      Infra                 `yaml:"infra"`

	// Path is the manifest that was loaded, Dir the directory relative paths
	// resolve against — never the working directory (D-9).
	Path string `yaml:"-"`
	Dir  string `yaml:"-"`
}

// Defaults are catalog-wide fallbacks a service may override.
type Defaults struct {
	LocalFiles LocalFileDefaults `yaml:"local_files"`
}

// LocalFileDefaults says where unversioned per-repo files live and how they
// are placed (D-10).
type LocalFileDefaults struct {
	Store string `yaml:"store"`
	Mode  string `yaml:"mode"`
}

// Ecosystem is one coherent set of services sharing a workspace and a network.
type Ecosystem struct {
	Name            string              `yaml:"-"`
	Workspace       string              `yaml:"workspace"`
	GitHost         string              `yaml:"git_host"`
	SSHKey          string              `yaml:"ssh_key"`
	DefaultWorktree string              `yaml:"default_worktree"`
	Compose         string              `yaml:"compose"`
	PortRange       []int               `yaml:"port_range"`
	Gateway         *Gateway            `yaml:"gateway"`
	Services        map[string]*Service `yaml:"services"`
}

// Gateway is the single entry point that mirrors the real routing (N-01).
type Gateway struct {
	Port   int               `yaml:"port"`
	Routes map[string]string `yaml:"routes"`
}

// Service is one runnable unit, of any runtime (D-6).
type Service struct {
	Name string `yaml:"-"`
	// Image marks a service that has no code of its own — a proxy, a
	// database, a ready-made image. It is built from nothing and has no
	// worktree, so the checks that look for code skip it.
	Image       string            `yaml:"image"`
	Repo        string            `yaml:"repo"`
	Runtime     string            `yaml:"runtime"`
	Worktree    string            `yaml:"worktree"`
	Build       map[string]any    `yaml:"build"`
	Port        Port              `yaml:"port"`
	Health      Health            `yaml:"health"`
	Needs       []string          `yaml:"needs"`
	Dev         *Dev              `yaml:"dev"`
	Infra       map[string]string `yaml:"infra"`
	LocalFiles  []LocalFile       `yaml:"local_files"`
	GatewayRoot bool              `yaml:"gateway_root"`
}

// Port separates what the service listens on from what the host may publish.
// Exposure is intent, not a mapping: a driver decides how to honour it (R-05).
type Port struct {
	Internal int `yaml:"internal"`
	Host     int `yaml:"host"`
}

// Health is how this service is probed. A dev server is not probed like a
// compiled binary, so the type is explicit (H-05).
type Health struct {
	Type    string `yaml:"type"`
	Path    string `yaml:"path"`
	Command string `yaml:"command"`
}

// Dev describes running the service with its code mounted and reloading (B-05).
type Dev struct {
	Mount   bool   `yaml:"mount"`
	Command string `yaml:"command"`
	Port    int    `yaml:"port"`
}

// LocalFile is an unversioned file a service needs: the original lives in the
// store, and it is placed where that service looks for it (S-02).
type LocalFile struct {
	From     string `yaml:"from"`
	To       string `yaml:"to"`
	Mode     string `yaml:"mode"`
	Required bool   `yaml:"required"`
}

// Infra holds the switchable infrastructure profiles (I-01, I-02).
type Infra struct {
	// Default is the profile used when a command does not name one. A
	// catalog whose services expect the machine's own Postgres should not
	// need everyone to remember a flag.
	Default  string                       `yaml:"default"`
	Profiles map[string]map[string]string `yaml:"profiles"`
}

// Known vocabularies. They are soft on purpose: an unknown value warns rather
// than fails, so a catalog is never blocked by this file being out of date.
var (
	KnownRuntimes    = []string{"go", "node", "angular", "react", "php", "python", "java", "static"}
	KnownHealthTypes = []string{"http", "tcp", "command"}
	LocalFileAnchors = []string{"repo:", "worktree:"}
	KnownModes       = []string{"link", "copy"}
)

// Components lists every infrastructure component named by any profile, so
// that a service may declare a need on one of them.
func (i Infra) Components() map[string]bool {
	out := map[string]bool{}
	for _, profile := range i.Profiles {
		for name := range profile {
			if name == "guard" {
				continue
			}
			out[name] = true
		}
	}
	return out
}

// WorktreeOf returns the worktree a service builds from: its own override, or
// the ecosystem default (G-01, G-02).
func (e *Ecosystem) WorktreeOf(s *Service) string {
	if s.Worktree != "" {
		return s.Worktree
	}
	return e.DefaultWorktree
}
