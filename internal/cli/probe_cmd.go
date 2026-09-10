// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/N3Y70R/runthrough/internal/probe"
	"github.com/N3Y70R/runthrough/internal/report"
	"github.com/N3Y70R/runthrough/internal/runner"
)

func runProbe(e *env, args []string) (*report.Result, error) {
	fs := e.flags("probe")
	eco := fs.String("eco", "", "ecosystem the services belong to")
	timeout := fs.Int("timeout", 5, "seconds to wait for an answer")
	all := fs.Bool("all", false, "probe every service in the ecosystem")
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
	if len(services) == 0 && !*all {
		return nil, fmt.Errorf("name a service, or pass --all (the ecosystem declares: %v)", p.Names())
	}
	if err := p.WriteEnvFile(); err != nil {
		return nil, err
	}

	ctx := context.Background()
	driver := runner.NewCompose()
	status := func(ctx context.Context) (map[string]probe.State, error) {
		states, err := driver.Status(ctx, p.Invocation())
		if err != nil {
			return nil, err
		}
		out := map[string]probe.State{}
		for name, s := range states {
			out[name] = probe.State{Running: s.State == "running", Health: s.Health}
		}
		return out, nil
	}

	results := probe.Wait(ctx, p.WaitTargets(services), time.Duration(*timeout)*time.Second, status)

	r := report.New("probe")
	for _, res := range results {
		switch {
		case res.Ready || res.Skipped:
			continue
		case res.Unverifiable:
			r.Fix("PR-002", report.Warning, p.Ecosystem+"/"+res.Service,
				"readiness cannot be verified from here", report.Remediation{Text: res.Detail, Fixable: false})
		default:
			// Probing is asking a question, and a service that does not
			// answer is the answer.
			r.Fix("PR-001", report.Error, p.Ecosystem+"/"+res.Service,
				fmt.Sprintf("no answer at %s within %ds", res.Target, *timeout),
				report.Remediation{
					Text:    "check whether it is running at all",
					Command: "runthrough status " + res.Service,
					Fixable: false,
				})
		}
	}
	r.Data = &probeData{Results: results}
	return r, nil
}

type probeData struct {
	Results []probe.Result `json:"results"`
}

func (d *probeData) WriteHuman(w io.Writer) error {
	fmt.Fprintf(w, "%-16s %-12s %s\n", "SERVICE", "RESULT", "PROBE")
	for _, res := range d.Results {
		result := "no answer"
		detail := res.Target
		switch {
		case res.Skipped:
			result, detail = "-", res.Detail
		case res.Unverifiable:
			result, detail = "unknown", res.Detail
		case res.Ready:
			result = fmt.Sprintf("ready %ds", res.Seconds)
			if detail == "" && res.Kind == probe.KindContainer {
				detail = "the container's own healthcheck"
			}
		}
		fmt.Fprintf(w, "%-16s %-12s %s\n", res.Service, result, detail)
	}
	return nil
}
