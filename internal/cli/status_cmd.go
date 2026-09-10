// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/N3Y70R/runthrough/internal/report"
	"github.com/N3Y70R/runthrough/internal/runner"
)

func runStatus(e *env, args []string) (*report.Result, error) {
	fs := e.flags("status")
	eco := fs.String("eco", "", "ecosystem to report on")
	services, err := parse(fs, args)
	if err != nil {
		return nil, err
	}
	p, _, err := loadPlan(e, *eco, "", setFlag{})
	if err != nil {
		return nil, err
	}
	if err := known(p, services); err != nil {
		return nil, err
	}
	if err := p.WriteEnvFile(); err != nil {
		return nil, err
	}

	ctx := context.Background()
	states, statusErr := runner.NewCompose().Status(ctx, p.Invocation())

	r := report.New("status")
	data := &statusData{Ecosystem: p.Ecosystem, Project: p.Project, Infra: p.Infra, Services: []serviceStatus{}}

	for _, s := range p.Services {
		if len(services) > 0 && !contains(services, s.Name) {
			continue
		}
		row := serviceStatus{
			Service:  s.Name,
			Worktree: s.Worktree,
			Branch:   s.Branch,
			Commit:   s.Commit,
			Dirty:    s.Dirty,
			HostPort: s.HostPort,
			State:    "not running",
		}
		if state, ok := states[s.Name]; ok {
			row.State = state.State
			row.Health = state.Health
			row.RunningCommit = state.Label(runner.LabelCommit)
			row.RunningBranch = state.Label(runner.LabelBranch)
		}
		// The whole point of the stack being an instrument: what is running
		// is not necessarily what the worktree holds now, and a difference
		// that goes unsaid is how an effect gets attributed to the wrong
		// change.
		if row.RunningCommit != "" && row.Commit != "" && row.RunningCommit != row.Commit {
			row.Drift = true
			r.Fix("ST-001", report.Warning, p.Ecosystem+"/"+s.Name,
				fmt.Sprintf("running %s but the worktree is at %s: what you are testing is not what you have", row.RunningCommit, row.Commit),
				report.Remediation{
					Text:    "rebuild it to test the code you actually have",
					Command: "runthrough up " + s.Name + " --build",
					Fixable: false,
				})
		}
		if row.State == "running" && row.Dirty {
			r.Addf("ST-002", report.Warning, p.Ecosystem+"/"+s.Name,
				"the worktree has uncommitted changes, so the running container matches no commit at all")
		}
		data.Services = append(data.Services, row)
	}

	if statusErr != nil {
		r.Fix("ST-003", report.Warning, p.Ecosystem, "could not ask the runtime what is running: "+statusErr.Error(),
			report.Remediation{Text: "the branches and commits below are what would be built, not what is up", Fixable: false})
	}

	r.Data = data
	return r, nil
}

type statusData struct {
	Ecosystem string          `json:"ecosystem"`
	Project   string          `json:"project"`
	Infra     string          `json:"infra"`
	Services  []serviceStatus `json:"services"`
}

type serviceStatus struct {
	Service       string `json:"service"`
	State         string `json:"state"`
	Health        string `json:"health,omitempty"`
	Worktree      string `json:"worktree,omitempty"`
	Branch        string `json:"branch,omitempty"`
	Commit        string `json:"commit,omitempty"`
	RunningBranch string `json:"running_branch,omitempty"`
	RunningCommit string `json:"running_commit,omitempty"`
	Dirty         bool   `json:"dirty,omitempty"`
	Drift         bool   `json:"drift,omitempty"`
	HostPort      int    `json:"host_port,omitempty"`
}

func (d *statusData) WriteHuman(w io.Writer) error {
	fmt.Fprintf(w, "%s · %s · infra %s\n\n", d.Ecosystem, d.Project, d.Infra)
	fmt.Fprintf(w, "%-16s %-12s %-10s %-11s %-11s %-7s %s\n",
		"SERVICE", "STATE", "HEALTH", "RUNNING", "WORKTREE", "PORT", "SOURCE")

	drift, dirty := false, false
	for _, s := range d.Services {
		running := dash(s.RunningCommit)
		if s.Drift {
			running += " ≠"
			drift = true
		}
		here := dash(s.Commit)
		if s.Dirty {
			here += "*"
			dirty = true
		}
		port := "-"
		if s.HostPort != 0 {
			port = fmt.Sprint(s.HostPort)
		}
		fmt.Fprintf(w, "%-16s %-12s %-10s %-11s %-11s %-7s %s\n",
			s.Service, s.State, dash(s.Health), running, here, port, dash(s.Branch))
	}
	if drift {
		fmt.Fprintln(w, "\n≠ the container was built from a different commit than the worktree holds now")
	}
	if dirty {
		fmt.Fprintln(w, "* uncommitted changes: what is built matches no commit")
	}
	return nil
}
