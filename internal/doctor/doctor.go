// SPDX-License-Identifier: GPL-3.0-or-later

// Package doctor answers "is this machine ready?" before anything is started.
//
// Every finding carries a stable code, a severity and, where possible, a
// remediation that says whether the tool can fix it itself (T-10, X-02). That
// shape is what lets an agent diagnose, explain and repair rather than guess
// from prose (D-13).
package doctor

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/N3Y70R/runthrough/internal/catalog"
	"github.com/N3Y70R/runthrough/internal/localfile"
	"github.com/N3Y70R/runthrough/internal/probe"
	"github.com/N3Y70R/runthrough/internal/report"
	"github.com/N3Y70R/runthrough/internal/runner"
	"github.com/N3Y70R/runthrough/internal/worktree"
)

// Options are the inputs of a check run.
type Options struct {
	Catalog   *catalog.Catalog
	Ecosystem string
	// Services narrows the check to these services. Asking about one service
	// and being blocked by another one's problem defeats the purpose of
	// checking a slice of the stack at a time.
	Services []string
	// Infra is the profile whose endpoints get checked. Empty means the
	// catalog's default.
	Infra string
	// Artifact and Env are the resolved driver artifact and the variables
	// the tool would pass it. With them, doctor can verify the contract the
	// artifact declares about itself — no catalog declaration needed.
	Artifact string
	Env      map[string]string
	Driver   runner.Driver
}

func (o Options) wants(name string) bool {
	if len(o.Services) == 0 {
		return true
	}
	for _, s := range o.Services {
		if s == name {
			return true
		}
	}
	return false
}

// Data is the structured outcome, and the human view of a run.
type Data struct {
	Manifest string          `json:"manifest"`
	Infra    string          `json:"infra"`
	Runtime  runner.Info     `json:"runtime"`
	Services []ServiceReport `json:"services"`
	Infras   []InfraReport   `json:"infrastructure,omitempty"`
	Ports    []PortReport    `json:"ports,omitempty"`
	// Requirements records presence only: no value of any secret ever
	// reaches this struct, the JSON output or the logs.
	Requirements []RequirementReport `json:"requirements,omitempty"`
	Artifact     *ArtifactReport     `json:"artifact,omitempty"`
}

// ArtifactReport is what the driver artifact needs and whether it has it.
type ArtifactReport struct {
	Path      string   `json:"path"`
	Variables int      `json:"variables"`
	Missing   []string `json:"missing,omitempty"`
	EnvFiles  int      `json:"env_files"`
}

// InfraReport is one infrastructure component and whether it answers.
type InfraReport struct {
	Component string `json:"component"`
	Mode      string `json:"mode"`
	Address   string `json:"address,omitempty"`
	Reachable bool   `json:"reachable"`
	Checked   bool   `json:"checked"`
}

// PortReport is one host port the ecosystem intends to publish.
type PortReport struct {
	Service string `json:"service"`
	Port    int    `json:"port"`
	Taken   bool   `json:"taken"`
}

// ServiceReport is what was found for one service.
type ServiceReport struct {
	Ecosystem  string            `json:"ecosystem"`
	Service    string            `json:"service"`
	Worktree   string            `json:"worktree"`
	Location   worktree.Location `json:"location"`
	LocalFiles []localfile.Plan  `json:"local_files,omitempty"`
}

// Run performs every check and returns one result.
func Run(ctx context.Context, o Options) *report.Result {
	r := report.New("doctor")
	data := &Data{Manifest: o.Catalog.Path}

	for _, f := range o.Catalog.Validate() {
		if len(o.Services) > 0 && !o.scoped(f.Scope) {
			continue
		}
		r.Add(f)
	}

	data.Runtime = o.Driver.Probe(ctx)
	checkRuntime(r, data.Runtime)

	env := newEnvSource(o.Catalog.Dir)
	checkRequires(r, env, "catalog", o.Catalog.Requires, nil, data)
	checkArtifact(r, o, data)

	infraName := o.Infra
	if infraName == "" {
		infraName = o.Catalog.Infra.Default
	}
	if infraName == "" {
		infraName = "local"
	}
	data.Infra = infraName
	profile := o.Catalog.Infra.Profiles[infraName]
	known := o.Catalog.Infra.Components()

	for _, ecoName := range sortedEcosystems(o.Catalog) {
		if o.Ecosystem != "" && o.Ecosystem != ecoName {
			continue
		}
		eco := o.Catalog.Ecosystems[ecoName]

		// Asking about a service means asking about everything it stands
		// on: checking only the literal names produces a clean diagnosis
		// followed by a failed start.
		names := o.Services
		if len(names) == 0 {
			names = eco.AllServices()
		}
		scope, components, unknown := eco.Closure(names, known)
		for _, name := range unknown {
			r.Fix("CFG-001", report.Error, ecoName, fmt.Sprintf("no service %q in this ecosystem", name),
				report.Remediation{Text: "the ecosystem declares: " + strings.Join(eco.AllServices(), ", "), Fixable: false})
		}

		locations := map[string]worktree.Location{}
		for _, svcName := range scope {
			svc := eco.Services[svcName]
			sr := checkService(ctx, r, o.Catalog, eco, svc)
			locations[svcName] = sr.Location
			data.Services = append(data.Services, sr)
		}

		for _, svcName := range scope {
			svc := eco.Services[svcName]
			loc := locations[svcName]
			checkRequires(r, env, ecoName+"/"+svcName, svc.Requires,
				map[string]string{"repo": loc.Repo, "worktree": loc.Path}, data)
		}

		checkInfra(r, o.Catalog, profile, infraName, components, data)
		checkPorts(r, eco, scope, data)
	}

	r.Data = data
	return r
}

// scoped reports whether a finding's scope belongs to the requested services.
// Findings that are not about a particular service always apply.
func (o Options) scoped(scope string) bool {
	if scope == "" {
		return true
	}
	name := scope
	if i := strings.LastIndex(scope, "/"); i >= 0 {
		name = scope[i+1:]
	}
	if name == scope {
		// An ecosystem-wide or defaults finding, not a service one.
		return true
	}
	return o.wants(name)
}

func checkRuntime(r *report.Result, info runner.Info) {
	if !info.Available {
		r.Fix("RT-001", report.Error, info.Driver, reasonOr(info, "the container runtime is not available"),
			report.Remediation{Text: "install the runtime and make sure its daemon is running", Fixable: false})
		return
	}
	if info.Compose == "" {
		r.Fix("RT-002", report.Error, info.Driver, reasonOr(info, "the compose plugin was not found"),
			report.Remediation{Text: "install the compose plugin for your runtime", Fixable: false})
		return
	}
	if info.Engine == "" {
		r.Fix("RT-003", report.Error, info.Driver, "the engine is not answering",
			report.Remediation{Text: "start the runtime daemon and try again", Fixable: false})
	}
	if !runner.AtLeast(info.Compose, runner.ComposeMinMajor, runner.ComposeMinMinor) {
		r.Fix("RT-004", report.Error, info.Driver,
			fmt.Sprintf("compose %s is below the supported floor %d.%d", info.Compose, runner.ComposeMinMajor, runner.ComposeMinMinor),
			report.Remediation{Text: "upgrade the compose plugin: the catalog relies on include:, which arrived in 2.20", Fixable: false})
	}
	if info.Engine != "" && !runner.AtLeast(info.Engine, runner.EngineMinMajor, runner.EngineMinMinor) {
		r.Fix("RT-005", report.Error, info.Driver,
			fmt.Sprintf("engine %s is below the supported floor %d.%d", info.Engine, runner.EngineMinMajor, runner.EngineMinMinor),
			report.Remediation{Text: "upgrade the engine: SSH build mounts need BuildKit as the default builder", Fixable: false})
	}
	// Capabilities, not version numbers, are what a run actually depends on.
	for _, cap := range runner.Required() {
		if !info.Supports(cap) {
			r.Fix("RT-006", report.Error, info.Driver,
				fmt.Sprintf("the runtime does not support %q, which the catalog relies on", cap),
				report.Remediation{Text: "upgrade the runtime, or use a driver that supports it", Fixable: false})
		}
	}
	if info.Experimental {
		r.Addf("RT-007", report.Warning, info.Driver, "this driver is experimental and has not been exercised in anger")
	}
}

// checkInfra verifies the components a service declares it needs, instead of
// assuming they are there. A stack whose database is not running starts
// cleanly and fails on the first query — long after the diagnosis said so.
func checkInfra(r *report.Result, cat *catalog.Catalog, profile map[string]string, profileName string, components []string, data *Data) {
	for _, component := range components {
		mode := profile[component]
		rep := InfraReport{Component: component, Mode: mode}
		switch mode {
		case "host":
			port := cat.Infra.PortOf(component)
			if port == 0 {
				r.Fix("IN-002", report.Warning, component, "no port known for this component, so it cannot be checked",
					report.Remediation{Text: "declare it under infra.ports in the catalog", Fixable: false})
				data.Infras = append(data.Infras, rep)
				continue
			}
			rep.Checked = true
			rep.Address = fmt.Sprintf("127.0.0.1:%d", port)
			if err := probe.Reachable("127.0.0.1", port); err != nil {
				r.Fix("IN-001", report.Error, component,
					fmt.Sprintf("nothing is listening on %s, but the %q profile expects it on this machine", rep.Address, profileName),
					report.Remediation{Text: "start it on your machine, or switch to a profile that runs it in a container (--infra local)", Fixable: false})
			} else {
				rep.Reachable = true
			}
		case "container":
			// The stack starts it; nothing to verify beforehand.
		case "remote", "":
			// Remote endpoints live in each service's own .env: the tool
			// does not know them and must not guess.
		}
		data.Infras = append(data.Infras, rep)
	}
}

// checkPorts reports host ports that are already taken, before a build that
// takes minutes ends in a collision.
func checkPorts(r *report.Result, eco *catalog.Ecosystem, scope []string, data *Data) {
	seen := map[int]bool{}
	check := func(name string, port int) {
		if port == 0 || seen[port] {
			return
		}
		seen[port] = true
		taken, family := probe.PortTaken(port)
		data.Ports = append(data.Ports, PortReport{Service: name, Port: port, Taken: taken})
		if !taken {
			return
		}
		r.Fix("PT-001", report.Warning, eco.Name+"/"+name,
			fmt.Sprintf("host port %d is already in use (%s)", port, family),
			report.Remediation{Text: "free the port, or change it in the catalog — if this stack is already running, this is expected", Fixable: false})
	}
	for _, name := range scope {
		if svc, ok := eco.Services[name]; ok {
			check(name, svc.Port.Host)
		}
	}
	if eco.Gateway != nil {
		check("gateway", eco.Gateway.Port)
	}
}

func checkService(ctx context.Context, r *report.Result, cat *catalog.Catalog, eco *catalog.Ecosystem, svc *catalog.Service) ServiceReport {
	scope := eco.Name + "/" + svc.Name
	if svc.Image != "" {
		return ServiceReport{Ecosystem: eco.Name, Service: svc.Name, Worktree: "-"}
	}
	want := eco.WorktreeOf(svc)
	loc := worktree.Resolve(ctx, eco.Workspace, svc.Repo, want)

	sr := ServiceReport{Ecosystem: eco.Name, Service: svc.Name, Worktree: want, Location: loc}

	switch {
	case loc.Layout == worktree.LayoutMissing && !dirExists(worktree.ExpandHome(eco.Workspace)):
		r.Fix("WT-001", report.Error, scope, fmt.Sprintf("the workspace %s does not exist", eco.Workspace),
			report.Remediation{Text: "create the workspace or correct it in the catalog", Fixable: false})
		return sr
	case loc.Layout == worktree.LayoutMissing:
		r.Fix("WT-002", report.Error, scope, fmt.Sprintf("no repository %s in the workspace", svc.Repo),
			report.Remediation{Text: "clone the repository into the workspace", Fixable: false})
		return sr
	case !loc.Exists:
		r.Fix("WT-003", report.Error, scope, fmt.Sprintf("the worktree %q is not checked out", want),
			report.Remediation{
				Text:    "create the worktree for that branch",
				Command: fmt.Sprintf("git -C %s worktree add %s %s", loc.Repo, want, want),
				Fixable: false,
			})
		return sr
	}

	if loc.Branch != "" && loc.Branch != want && loc.Layout == worktree.LayoutPlain {
		r.Addf("WT-004", report.Warning, scope, "the clone is on branch %q but the catalog asks for %q", loc.Branch, want)
	}
	if loc.Dirty {
		r.Addf("WT-005", report.Warning, scope, "the worktree has uncommitted changes: what you build will not match any commit")
	}
	if loc.GitError != "" {
		r.Addf("WT-006", report.Warning, scope, "could not read git state: %s", loc.GitError)
	}

	lfCtx := localfile.Context{
		Store:        cat.Defaults.LocalFiles.Store,
		Ecosystem:    eco.Name,
		Service:      svc.Name,
		RepoPath:     loc.Repo,
		WorktreePath: loc.Path,
	}
	for _, decl := range svc.LocalFiles {
		plan := localfile.Resolve(lfCtx, decl)
		sr.LocalFiles = append(sr.LocalFiles, plan)
		reportLocalFile(r, scope, plan)
	}
	return sr
}

func reportLocalFile(r *report.Result, scope string, p localfile.Plan) {
	switch p.Status {
	case localfile.StatusOK:
		return
	case localfile.StatusMissingSource:
		severity := report.Warning
		if p.Required {
			severity = report.Error
		}
		r.Fix("LF-001", severity, scope, fmt.Sprintf("the original %s is missing from the store", p.From),
			report.Remediation{
				Text:    "ask your team for this file and put it in the store — the tool will not invent it, because inventing it would mean inventing secrets",
				Fixable: false,
			})
	case localfile.StatusMissingTarget:
		ignored := gitIgnores(p.To)
		rem := report.Remediation{
			Text:    fmt.Sprintf("link %s to the original in the store", p.To),
			Command: "runthrough doctor --fix",
			Fixable: true,
		}
		if !ignored {
			rem.Text = "the target is not ignored by git: fix that first, or the tool would risk making a secret committable"
			rem.Command = ""
			rem.Fixable = false
			r.Fix("LF-002", report.Error, scope, fmt.Sprintf("%s is missing and its path is not git-ignored", p.To), rem)
			return
		}
		r.Fix("LF-003", report.Warning, scope, fmt.Sprintf("%s is not in place", p.To), rem)
	case localfile.StatusForeign:
		r.Fix("LF-004", report.Warning, scope, fmt.Sprintf("%s already exists and is not a link to the store", p.To),
			report.Remediation{Text: "move it into the store and let the tool link it — nothing is ever overwritten", Fixable: false})
	case localfile.StatusMismatch:
		detail := p.Detail
		if detail != "" {
			detail = " (" + detail + ")"
		}
		r.Fix("LF-005", report.Warning, scope, fmt.Sprintf("%s links somewhere other than the store%s", p.To, detail),
			report.Remediation{Text: "remove the link and let the tool recreate it, or correct the store in the catalog", Fixable: false})
	}
}

// gitIgnores reports whether git would ignore a path. A path git tracks must
// never receive a generated link (S-06).
func gitIgnores(path string) bool {
	dir := parentDir(path)
	cmd := exec.Command("git", "-C", dir, "check-ignore", "-q", path)
	return cmd.Run() == nil
}

func parentDir(path string) string {
	dir := path
	for i := len(dir) - 1; i >= 0; i-- {
		if dir[i] == os.PathSeparator {
			return dir[:i]
		}
	}
	return "."
}

func reasonOr(i runner.Info, fallback string) string {
	if i.Reason != "" {
		return i.Reason
	}
	return fallback
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func sortedEcosystems(c *catalog.Catalog) []string {
	out := make([]string, 0, len(c.Ecosystems))
	for k := range c.Ecosystems {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedServices(e *catalog.Ecosystem) []string {
	out := make([]string, 0, len(e.Services))
	for k := range e.Services {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// WriteHuman renders the run for people: the runtime first, then a row per
// service with the branch and commit that would be built.
func (d *Data) WriteHuman(w io.Writer) error {
	rt := d.Runtime
	status := "not available"
	if rt.Available {
		status = "ready"
	}
	fmt.Fprintf(w, "runtime        %s (%s)\n", rt.Driver, status)
	if d.Infra != "" {
		fmt.Fprintf(w, "infra profile  %s\n", d.Infra)
	}
	if rt.Client != "" {
		fmt.Fprintf(w, "versions       client %s, engine %s, compose %s\n", dash(rt.Client), dash(rt.Engine), dash(rt.Compose))
	}
	if len(rt.Capabilities) > 0 {
		var missing []string
		for _, c := range runner.Required() {
			if !rt.Supports(c) {
				missing = append(missing, string(c))
			}
		}
		if len(missing) == 0 {
			fmt.Fprintf(w, "capabilities   all %d present\n", len(runner.Required()))
		} else {
			fmt.Fprintf(w, "capabilities   missing %v\n", missing)
		}
	}
	if rt.Reason != "" {
		fmt.Fprintf(w, "note           %s\n", rt.Reason)
	}

	if len(d.Services) == 0 {
		return nil
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%-22s %-14s %-16s %-9s %s\n", "SERVICE", "WORKTREE", "BRANCH", "COMMIT", "FILES")
	for _, s := range d.Services {
		branch := dash(s.Location.Branch)
		if s.Location.Dirty {
			branch += "*"
		}
		fmt.Fprintf(w, "%-22s %-14s %-16s %-9s %s\n",
			s.Ecosystem+"/"+s.Service,
			s.Worktree,
			branch,
			dash(s.Location.Commit),
			filesSummary(s.LocalFiles),
		)
	}
	for _, s := range d.Services {
		if s.Location.Dirty {
			fmt.Fprintln(w, "\n* uncommitted changes: what is built will not match any commit")
			break
		}
	}
	if len(d.Infras) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "%-14s %-10s %s\n", "COMPONENT", "MODE", "STATE")
		for _, i := range d.Infras {
			state := "not checked"
			switch {
			case i.Checked && i.Reachable:
				state = "answering at " + i.Address
			case i.Checked:
				state = "NOT answering at " + i.Address
			case i.Mode == "container":
				state = "started with the stack"
			case i.Mode == "remote":
				state = "remote, from the service's own .env"
			}
			fmt.Fprintf(w, "%-14s %-10s %s\n", i.Component, dash(i.Mode), state)
		}
	}
	return nil
}

func filesSummary(plans []localfile.Plan) string {
	if len(plans) == 0 {
		return "-"
	}
	ok := 0
	for _, p := range plans {
		if p.Status == localfile.StatusOK {
			ok++
		}
	}
	return fmt.Sprintf("%d/%d", ok, len(plans))
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
