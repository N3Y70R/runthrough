// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/N3Y70R/runthrough/internal/catalog"
	"github.com/N3Y70R/runthrough/internal/config"
	"github.com/N3Y70R/runthrough/internal/doctor"
	"github.com/N3Y70R/runthrough/internal/plan"
	"github.com/N3Y70R/runthrough/internal/report"
	"github.com/N3Y70R/runthrough/internal/runner"
)

// setFlag collects repeated --set service=branch pairs (G-02).
type setFlag map[string]string

func (s setFlag) String() string { return "" }

func (s setFlag) Set(v string) error {
	name, branch, ok := strings.Cut(v, "=")
	if !ok || name == "" || branch == "" {
		return fmt.Errorf("expected service=branch, got %q", v)
	}
	s[name] = branch
	return nil
}

func loadPlan(e *env, eco, infra string, set setFlag) (*plan.Plan, *catalog.Catalog, error) {
	lookup, _, err := e.lookup()
	if err != nil {
		return nil, nil, err
	}
	res, err := config.ResolveCatalog(lookup)
	if err != nil {
		return nil, nil, err
	}
	cat, err := catalog.Load(res.Path)
	if err != nil {
		return nil, nil, err
	}
	p, err := plan.Build(context.Background(), cat, plan.Options{Ecosystem: eco, Infra: infra, Worktrees: set})
	if err != nil {
		return nil, nil, err
	}
	return p, cat, nil
}

func runUp(e *env, args []string) (*report.Result, error) {
	fs := e.flags("up")
	eco := fs.String("eco", "", "ecosystem to bring up")
	infra := fs.String("infra", "", "infrastructure profile: local, host or shared")
	build := fs.Bool("build", false, "build images before starting")
	skip := fs.Bool("skip-checks", false, "start without running the readiness checks first")
	set := setFlag{}
	fs.Var(set, "set", "pin a service to a branch, as service=branch (repeatable)")
	services, err := parse(fs, args)
	if err != nil {
		return nil, err
	}

	p, cat, err := loadPlan(e, *eco, *infra, set)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	driver := runner.NewCompose()

	r := report.New("up")
	if !*skip {
		checks := doctor.Run(ctx, doctor.Options{Catalog: cat, Ecosystem: p.Ecosystem, Services: services, Driver: driver})
		for _, f := range checks.Findings {
			r.Add(f)
		}
		if r.HasErrors() {
			r.Data = &upData{Plan: p, Requested: services, Started: false, Note: "nothing was started: the readiness checks found errors. Fix them, or pass --skip-checks to start anyway."}
			return r, nil
		}
	}
	for _, s := range p.Missing() {
		if len(services) > 0 && !contains(services, s.Name) {
			continue
		}
		r.Addf("UP-001", report.Error, p.Ecosystem+"/"+s.Name, "the worktree %q of %s is not on disk", s.Worktree, s.Repo)
	}
	if r.HasErrors() {
		r.Data = &upData{Plan: p, Requested: services, Started: false, Note: "nothing was started: some services have no code to build from."}
		return r, nil
	}

	if err := p.WriteEnvFile(); err != nil {
		return nil, err
	}
	err = driver.Up(ctx, p.Invocation(), runner.UpOptions{Services: services, Build: *build}, e.stdout, e.stderr)
	if err != nil {
		r.Fix("UP-002", report.Error, p.Ecosystem, fmt.Sprintf("the runtime refused to bring the stack up: %v", err),
			report.Remediation{
				Text:    "reproduce it by hand with the same arguments the tool used",
				Command: driver.Command(p.Invocation(), append([]string{"up", "--detach"}, services...)),
				Fixable: false,
			})
	}
	r.Data = &upData{Plan: p, Requested: services, Started: err == nil}
	return r, nil
}

func runDown(e *env, args []string) (*report.Result, error) {
	fs := e.flags("down")
	eco := fs.String("eco", "", "ecosystem to take down")
	volumes := fs.Bool("volumes", false, "also delete the volumes: this destroys local data")
	if _, err := parse(fs, args); err != nil {
		return nil, err
	}
	p, _, err := loadPlan(e, *eco, "", setFlag{})
	if err != nil {
		return nil, err
	}
	if err := p.WriteEnvFile(); err != nil {
		return nil, err
	}
	driver := runner.NewCompose()
	r := report.New("down")
	if err := driver.Down(context.Background(), p.Invocation(), runner.DownOptions{Volumes: *volumes}, e.stderr, e.stderr); err != nil {
		r.Addf("DN-001", report.Error, p.Ecosystem, "the runtime refused to take the stack down: %v", err)
	}
	r.Data = &simpleData{Line: fmt.Sprintf("stack %s stopped (volumes kept: %v)", p.Project, !*volumes)}
	return r, nil
}

func runRebuild(e *env, args []string) (*report.Result, error) {
	fs := e.flags("rebuild")
	eco := fs.String("eco", "", "ecosystem the service belongs to")
	set := setFlag{}
	fs.Var(set, "set", "pin a service to a branch, as service=branch (repeatable)")
	services, err := parse(fs, args)
	if err != nil {
		return nil, err
	}
	p, _, err := loadPlan(e, *eco, "", set)
	if err != nil {
		return nil, err
	}
	if err := p.WriteEnvFile(); err != nil {
		return nil, err
	}
	r := report.New("rebuild")
	if err := runner.NewCompose().Build(context.Background(), p.Invocation(), services, e.stderr, e.stderr); err != nil {
		r.Addf("RB-001", report.Error, p.Ecosystem, "the build failed: %v", err)
	}
	r.Data = &simpleData{Line: fmt.Sprintf("rebuilt %s", strings.Join(defaultTo(services, []string{"every service"}), ", "))}
	return r, nil
}

func runLogs(e *env, args []string) (*report.Result, error) {
	fs := e.flags("logs")
	eco := fs.String("eco", "", "ecosystem to read logs from")
	follow := fs.Bool("f", false, "follow the output")
	services, err := parse(fs, args)
	if err != nil {
		return nil, err
	}
	p, _, err := loadPlan(e, *eco, "", setFlag{})
	if err != nil {
		return nil, err
	}
	if err := p.WriteEnvFile(); err != nil {
		return nil, err
	}
	r := report.New("logs")
	if err := runner.NewCompose().Logs(context.Background(), p.Invocation(), services, *follow, e.stdout, e.stderr); err != nil {
		r.Addf("LG-001", report.Warning, p.Ecosystem, "the log stream ended: %v", err)
	}
	// logs writes the services' own output; a trailing "no findings" line
	// would read as part of it.
	r.Data = quietData{}
	return r, nil
}

func runPlan(e *env, args []string) (*report.Result, error) {
	fs := e.flags("plan")
	eco := fs.String("eco", "", "ecosystem to resolve")
	infra := fs.String("infra", "", "infrastructure profile: local, host or shared")
	set := setFlag{}
	fs.Var(set, "set", "pin a service to a branch, as service=branch (repeatable)")
	if _, err := parse(fs, args); err != nil {
		return nil, err
	}
	p, _, err := loadPlan(e, *eco, *infra, set)
	if err != nil {
		return nil, err
	}
	if err := p.WriteEnvFile(); err != nil {
		return nil, err
	}
	r := report.New("plan")
	r.Data = &planData{Plan: p, Command: runner.NewCompose().Command(p.Invocation(), []string{"up", "--detach"})}
	return r, nil
}

type upData struct {
	Plan      *plan.Plan `json:"plan"`
	Requested []string   `json:"requested,omitempty"`
	Started   bool       `json:"started"`
	Note      string     `json:"note,omitempty"`
}

func (d *upData) WriteHuman(w io.Writer) error {
	// Report what was asked for, never the size of the catalog: claiming
	// four services are up when one container is running is a lie the reader
	// has no way to catch.
	what := "every service in the ecosystem"
	if len(d.Requested) > 0 {
		what = strings.Join(d.Requested, ", ")
	}
	if d.Started {
		fmt.Fprintf(w, "%s: started %s (infra %s)\n\n", d.Plan.Project, what, d.Plan.Infra)
	} else {
		fmt.Fprintf(w, "%s: nothing started (%s was requested)\n\n", d.Plan.Project, what)
	}
	writeServices(w, d.Plan, d.Requested)
	if d.Note != "" {
		fmt.Fprintf(w, "\n%s\n", d.Note)
	}
	return nil
}

type quietData struct{}

func (quietData) WriteHuman(io.Writer) error { return nil }

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

type planData struct {
	Plan    *plan.Plan `json:"plan"`
	Command string     `json:"command"`
}

func (d *planData) WriteHuman(w io.Writer) error {
	fmt.Fprintf(w, "ecosystem      %s\ninfrastructure %s\nproject        %s\nartifact       %s\n\n",
		d.Plan.Ecosystem, d.Plan.Infra, d.Plan.Project, d.Plan.File)
	writeServices(w, d.Plan, nil)
	fmt.Fprintf(w, "\nby hand:\n  %s\n", d.Command)
	return nil
}

type simpleData struct {
	Line string `json:"message"`
}

func (d *simpleData) WriteHuman(w io.Writer) error {
	_, err := fmt.Fprintln(w, d.Line)
	return err
}

func writeServices(w io.Writer, p *plan.Plan, only []string) {
	dirty := false
	fmt.Fprintf(w, "%-16s %-14s %-16s %-9s %-7s %s\n", "SERVICE", "WORKTREE", "BRANCH", "COMMIT", "PORT", "CONTEXT")
	for _, s := range p.Services {
		if len(only) > 0 && !contains(only, s.Name) {
			continue
		}
		if s.Dirty {
			dirty = true
		}
		branch := s.Branch
		if branch == "" {
			branch = "-"
		}
		if s.Dirty {
			branch += "*"
		}
		port := "-"
		if s.HostPort != 0 {
			port = fmt.Sprintf("%d", s.HostPort)
		}
		commit := s.Commit
		if commit == "" {
			commit = "-"
		}
		fmt.Fprintf(w, "%-16s %-14s %-16s %-9s %-7s %s\n", s.Name, s.Worktree, branch, commit, port, s.Context)
	}
	if dirty {
		fmt.Fprintln(w, "\n* uncommitted changes: what is built will not match any commit")
	}
}

func defaultTo(v, fallback []string) []string {
	if len(v) == 0 {
		return fallback
	}
	return v
}
