// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"

	"github.com/N3Y70R/runthrough/internal/catalog"
	"github.com/N3Y70R/runthrough/internal/config"
	"github.com/N3Y70R/runthrough/internal/doctor"
	"github.com/N3Y70R/runthrough/internal/report"
	"github.com/N3Y70R/runthrough/internal/runner"
)

func runDoctor(e *env, args []string) (*report.Result, error) {
	fs := e.flags("doctor")
	eco := fs.String("eco", "", "check only this ecosystem")
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

	return doctor.Run(context.Background(), doctor.Options{
		Catalog:   cat,
		Ecosystem: *eco,
		Services:  services,
		Driver:    runner.NewCompose(),
	}), nil
}
