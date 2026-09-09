// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"

	"github.com/N3Y70R/runthrough/internal/catalog"
	"github.com/N3Y70R/runthrough/internal/config"
	"github.com/N3Y70R/runthrough/internal/doctor"
	"github.com/N3Y70R/runthrough/internal/plan"
	"github.com/N3Y70R/runthrough/internal/report"
	"github.com/N3Y70R/runthrough/internal/runner"
)

func runDoctor(e *env, args []string) (*report.Result, error) {
	fs := e.flags("doctor")
	eco := fs.String("eco", "", "check only this ecosystem")
	infra := fs.String("infra", "", "infrastructure profile to check against")
	services, err := parse(fs, args)
	if err != nil {
		return nil, err
	}

	lookup, _, err := e.lookup()
	if err != nil {
		return nil, err
	}
	res, err := config.ResolveCatalog(lookup)
	if err != nil {
		return nil, err
	}
	cat, err := catalog.Load(res.Path)
	if err != nil {
		return nil, err
	}

	opts := doctor.Options{
		Catalog:   cat,
		Ecosystem: *eco,
		Services:  services,
		Infra:     *infra,
		Driver:    runner.NewCompose(),
	}
	// Resolving the plan is what lets doctor read the artifact's own
	// contract. It is best effort: a catalog that cannot be resolved is
	// itself reported by the checks below.
	if p, err := plan.Build(context.Background(), cat, plan.Options{Ecosystem: *eco, Infra: *infra}); err == nil {
		opts.Artifact = p.File
		opts.Env = p.Env
	}

	return doctor.Run(context.Background(), opts), nil
}
