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
	"sort"
	"strconv"
	"strings"

	"github.com/N3Y70R/runthrough/internal/catalog"
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
			s := Service{Name: name, Image: svc.Image, HostPort: svc.Port.Host, Port: svc.Port.Internal, Exists: true}
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

// Invocation hands the plan to a driver.
func (p *Plan) Invocation() runner.Invocation {
	return runner.Invocation{Project: p.Project, File: p.File, Dir: p.Dir, Env: p.Env}
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
