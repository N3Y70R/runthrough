// SPDX-License-Identifier: GPL-3.0-or-later

// Package plan turns the catalog into the concrete environment a driver runs
// with: which worktree each service builds from, which ports it publishes,
// where its infrastructure lives, and what provenance to stamp on its image.
//
// This is where D-2 lives: the driver artifact stays versioned and readable,
// and the variables it interpolates are resolved here.
package plan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/N3Y70R/runthrough/internal/artifact"
	"github.com/N3Y70R/runthrough/internal/catalog"
	"github.com/N3Y70R/runthrough/internal/probe"
	"github.com/N3Y70R/runthrough/internal/runner"
	"github.com/N3Y70R/runthrough/internal/worktree"
)

// HostAlias is the name a container uses to reach the machine it runs on. It
// is what makes "the service I am editing runs outside" possible (N-06).
const HostAlias = "host.docker.internal"

// Service is one resolved service.
type Service struct {
	Name     string `json:"name"`
	Image    string `json:"image,omitempty"`
	Repo     string `json:"repo,omitempty"`
	Worktree string `json:"worktree"`
	Context  string `json:"context"`
	RepoPath string `json:"repo_path"`
	Branch   string `json:"branch,omitempty"`
	Commit   string `json:"commit,omitempty"`
	Dirty    bool   `json:"dirty,omitempty"`
	HostPort int    `json:"host_port,omitempty"`
	Port     int    `json:"port,omitempty"`
	Health   string `json:"health,omitempty"`
	Path     string `json:"health_path,omitempty"`
	Exists   bool   `json:"exists"`
}

// Plan is the resolved ecosystem.
type Plan struct {
	Ecosystem string            `json:"ecosystem"`
	Project   string            `json:"project"`
	File      string            `json:"file"`
	Dir       string            `json:"dir"`
	Infra     string            `json:"infra"`
	Services  []Service         `json:"services"`
	Env       map[string]string `json:"env"`
	EnvFile   string            `json:"env_file,omitempty"`
	// Artifact is what the driver file says about itself. The catalog
	// declares intent; the artifact decides what actually runs.
	Artifact *artifact.Contract `json:"-"`
}

// Options are the inputs that vary between runs.
type Options struct {
	Ecosystem string
	Infra     string
	// Worktrees overrides the branch a given service builds from, which is
	// the whole point of G-02: one service on a feature branch, the rest on
	// the release branch.
	Worktrees map[string]string
}

// Build resolves everything the driver needs. It fails only on things that
// make a run impossible; anything merely suspicious is doctor's job to report.
func Build(ctx context.Context, cat *catalog.Catalog, o Options) (*Plan, error) {
	ecoName := o.Ecosystem
	if ecoName == "" {
		if len(cat.Ecosystems) != 1 {
			return nil, fmt.Errorf("the catalog holds %d ecosystems: name one with --eco", len(cat.Ecosystems))
		}
		for name := range cat.Ecosystems {
			ecoName = name
		}
	}
	eco, ok := cat.Ecosystems[ecoName]
	if !ok {
		return nil, fmt.Errorf("no ecosystem %q in this catalog", ecoName)
	}
	if eco.Compose == "" {
		return nil, fmt.Errorf("ecosystem %q declares no driver artifact (compose:)", ecoName)
	}

	infra := o.Infra
	if infra == "" {
		infra = cat.Infra.Default
	}
	if infra == "" {
		infra = "local"
	}
	if _, ok := cat.Infra.Profiles[infra]; !ok && len(cat.Infra.Profiles) > 0 {
		return nil, fmt.Errorf("no infrastructure profile %q: the catalog declares %s", infra, strings.Join(profileNames(cat), ", "))
	}

	p := &Plan{
		Ecosystem: ecoName,
		Project:   "rt-" + ecoName,
		File:      cat.Resolve(eco.Compose),
		Dir:       cat.Dir,
		Infra:     infra,
		Env:       map[string]string{},
	}

	if contract, err := artifact.Read(p.File); err == nil {
		p.Artifact = contract
	}

	p.Env["RT_PROJECT"] = p.Project
	p.Env["RT_ECO"] = ecoName
	p.Env["RT_INFRA"] = infra
	p.Env["RT_HOST_ALIAS"] = HostAlias
	p.Env["RT_WORKSPACE"] = worktree.ExpandHome(eco.Workspace)
	if eco.Gateway != nil && eco.Gateway.Port != 0 {
		p.Env["RT_GATEWAY_PORT"] = strconv.Itoa(eco.Gateway.Port)
	}

	for component, mode := range cat.Infra.Profiles[infra] {
		if component == "guard" {
			p.Env["RT_INFRA_GUARD"] = mode
			continue
		}
		key := "RT_INFRA_" + envName(component)
		p.Env[key+"_MODE"] = mode
		switch mode {
		case "container":
			p.Env[key+"_HOST"] = component
		case "host":
			p.Env[key+"_HOST"] = HostAlias
		}
		// A remote component keeps whatever the service's own .env says:
		// the tool does not know, and must not invent, remote endpoints.
	}

	for _, name := range sortedServices(eco) {
		svc := eco.Services[name]
		prefixEarly := "RT_" + envName(name)
		if svc.Image != "" {
			// No code, no worktree: a ready-made image only needs its ports.
			s := Service{Name: name, Image: svc.Image, HostPort: svc.Port.Host, Port: svc.Port.Internal,
				Health: svc.Health.Type, Path: svc.Health.Path, Exists: true}
			p.Services = append(p.Services, s)
			p.Env[prefixEarly+"_IMAGE"] = svc.Image
			if s.Port != 0 {
				p.Env[prefixEarly+"_PORT"] = strconv.Itoa(s.Port)
			}
			if s.HostPort != 0 {
				p.Env[prefixEarly+"_HOST_PORT"] = strconv.Itoa(s.HostPort)
			}
			continue
		}
		want := eco.WorktreeOf(svc)
		if override, ok := o.Worktrees[name]; ok && override != "" {
			want = override
		}
		loc := worktree.Resolve(ctx, eco.Workspace, svc.Repo, want)

		s := Service{
			Name:     name,
			Repo:     svc.Repo,
			Worktree: want,
			Context:  loc.Path,
			RepoPath: loc.Repo,
			Branch:   loc.Branch,
			Commit:   loc.Commit,
			Dirty:    loc.Dirty,
			HostPort: svc.Port.Host,
			Port:     svc.Port.Internal,
			Health:   svc.Health.Type,
			Path:     svc.Health.Path,
			Exists:   loc.Exists,
		}
		p.Services = append(p.Services, s)

		prefix := "RT_" + envName(name)
		p.Env[prefix+"_CONTEXT"] = s.Context
		p.Env[prefix+"_REPO"] = s.RepoPath
		p.Env[prefix+"_WORKTREE"] = s.Worktree
		p.Env[prefix+"_BRANCH"] = s.Branch
		p.Env[prefix+"_COMMIT"] = s.Commit
		if s.Port != 0 {
			p.Env[prefix+"_PORT"] = strconv.Itoa(s.Port)
		}
		if s.HostPort != 0 {
			p.Env[prefix+"_HOST_PORT"] = strconv.Itoa(s.HostPort)
		}
		if svc.Dev != nil {
			p.Env[prefix+"_DEV_COMMAND"] = svc.Dev.Command
			if svc.Dev.Port != 0 {
				p.Env[prefix+"_DEV_PORT"] = strconv.Itoa(svc.Dev.Port)
			}
		}
	}

	return p, nil
}

// RemoveEnvFile deletes the generated env-file. It holds whatever the
// catalog's .env held, so it should not outlive the stack it was written for.
func (p *Plan) RemoveEnvFile() error {
	path := p.EnvFile
	if path == "" {
		dir, err := os.UserCacheDir()
		if err != nil {
			dir = os.TempDir()
		}
		path = filepath.Join(dir, "runthrough", p.Project+".env")
	}
	return os.Remove(path)
}

// Invocation hands the plan to a driver.
func (p *Plan) Invocation() runner.Invocation {
	return runner.Invocation{Project: p.Project, File: p.File, Dir: p.Dir, Env: p.Env, EnvFile: p.EnvFile}
}

// WriteEnvFile persists the resolved variables next to the user's cache and
// records the path on the plan.
//
// This is what makes the escape hatch true rather than aspirational: the
// driver is invoked with --env-file, so the command the tool prints is the
// command a person can paste. Before this, the printed command failed with
// "variable is not set" because the values only lived in the child process.
func (p *Plan) WriteEnvFile() error {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "runthrough")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, p.Project+".env")

	keys := make([]string, 0, len(p.Env))
	for k := range p.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("# Generated by runthrough for project " + p.Project + ".\n")
	b.WriteString("# Resolved from the catalog; edit the catalog, not this file.\n")

	// Passing --env-file replaces the driver's own loading of the catalog's
	// .env, so its contents are carried over here first. Without this, a
	// value the catalog expected — a shared signing secret, say — would go
	// quietly missing the moment the tool started passing an env-file.
	if carried, err := readEnvFile(filepath.Join(p.Dir, ".env")); err == nil && len(carried) > 0 {
		b.WriteString("\n# Carried over from the .env next to the catalog.\n")
		for _, line := range carried {
			b.WriteString(line + "\n")
		}
		b.WriteString("\n# Resolved by runthrough.\n")
	}

	for _, k := range keys {
		b.WriteString(k + "=" + p.Env[k] + "\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return err
	}
	p.EnvFile = path
	return nil
}

// WaitTargets turns the plan into things to poll after starting. A service
// with no published port cannot be reached from the host: that is a skip with
// a reason, not a failure.
func (p *Plan) WaitTargets(only []string) []probe.Target {
	var out []probe.Target
	for _, s := range p.Services {
		if len(only) > 0 && !containsString(only, s.Name) {
			continue
		}
		t := probe.Target{Service: s.Name}

		declared, _ := p.artifactService(s.Name)
		switch {
		case declared.Healthcheck:
			// The runtime's own healthcheck runs inside the container, so
			// it cannot be satisfied by a port the runtime opened first.
			t.Kind = probe.KindContainer
		case s.HostPort == 0:
			t.Skip = "no published port: not reachable from this machine"
		case s.Health == "http":
			t.Kind = probe.KindHTTP
			path := s.Path
			if path == "" {
				path = "/"
			}
			t.URL = fmt.Sprintf("http://localhost:%d%s", s.HostPort, path)
		default:
			// A TCP connect against a published port always succeeds: the
			// runtime answers it before the service does. Reporting that as
			// ready would be a false promise, which is worse than no
			// promise at all.
			t.Kind = probe.KindUnverifiable
			t.Reason = fmt.Sprintf("a tcp probe against published port %d proves nothing: the runtime answers it before the service does. Declare health.type http, or a healthcheck in the artifact", s.HostPort)
		}
		out = append(out, t)
	}
	return out
}

func (p *Plan) artifactService(name string) (artifact.Service, bool) {
	if p.Artifact == nil {
		return artifact.Service{}, false
	}
	return p.Artifact.Service(name)
}

// Names lists every service in the plan.
func (p *Plan) Names() []string {
	out := make([]string, 0, len(p.Services))
	for _, s := range p.Services {
		out = append(out, s.Name)
	}
	return out
}

// Buildable splits names into those that build from source and those that
// only pull an image.
// Buildable splits names by what the ARTIFACT declares, not by what the
// catalog intends: a service the catalog describes as source may still run a
// ready-made image, and only the artifact knows.
func (p *Plan) Buildable(names []string) (build, imageOnly []string) {
	for _, s := range p.Services {
		if len(names) > 0 && !containsString(names, s.Name) {
			continue
		}
		if declared, ok := p.artifactService(s.Name); ok {
			if declared.Builds {
				build = append(build, s.Name)
			} else {
				imageOnly = append(imageOnly, s.Name)
			}
			continue
		}
		if s.Image != "" {
			imageOnly = append(imageOnly, s.Name)
			continue
		}
		build = append(build, s.Name)
	}
	return build, imageOnly
}

// readEnvFile returns the assignment lines of an env file, untouched. The
// tool moves them; it does not read, log or interpret what they hold.
func readEnvFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.Contains(trimmed, "=") {
			continue
		}
		out = append(out, trimmed)
	}
	return out, nil
}

func containsString(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

// Missing lists services whose code is not on disk. Starting without them is
// not a warning, it is a different stack.
func (p *Plan) Missing() []Service {
	var out []Service
	for _, s := range p.Services {
		if !s.Exists {
			out = append(out, s)
		}
	}
	return out
}

// envName turns a service or component name into a variable name.
func envName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

func sortedServices(e *catalog.Ecosystem) []string {
	out := make([]string, 0, len(e.Services))
	for k := range e.Services {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func profileNames(c *catalog.Catalog) []string {
	out := make([]string, 0, len(c.Infra.Profiles))
	for k := range c.Infra.Profiles {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
