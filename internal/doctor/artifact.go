// SPDX-License-Identifier: GPL-3.0-or-later

package doctor

import (
	"fmt"
	"os"

	"github.com/N3Y70R/runthrough/internal/artifact"
	"github.com/N3Y70R/runthrough/internal/report"
)

// checkArtifact verifies the contract the driver artifact declares about
// itself: the variables it interpolates without a default, and the env files
// it loads.
//
// Nothing here has to be declared in the catalog. The artifact already said
// what it needs; this reads it. It is the difference between failing before
// anything runs and failing halfway through a build, which is where the same
// problem surfaced in testing.
func checkArtifact(r *report.Result, o Options, data *Data) {
	if o.Artifact == "" {
		return
	}
	contract, err := artifact.Read(o.Artifact)
	if err != nil && contract == nil {
		r.Fix("AR-003", report.Error, "artifact", fmt.Sprintf("cannot read the driver artifact: %v", err),
			report.Remediation{Text: "check the compose file named by the ecosystem", Fixable: false})
		return
	}
	if err != nil {
		r.Addf("AR-004", report.Warning, "artifact", "the artifact could not be parsed in full: %v", err)
	}

	rep := &ArtifactReport{Path: o.Artifact, Variables: len(contract.Variables), EnvFiles: len(contract.EnvFiles)}

	resolved := func(name string) bool {
		if _, ok := o.Env[name]; ok {
			return true
		}
		if _, ok := os.LookupEnv(name); ok {
			return true
		}
		return false
	}

	for _, v := range contract.Missing(resolved) {
		rep.Missing = append(rep.Missing, v.Name)
		msg := fmt.Sprintf("the artifact interpolates ${%s} with no default, and nothing defines it", v.Name)
		if v.Explicit && v.Message != "" {
			msg = fmt.Sprintf("${%s} is required by the artifact: %s", v.Name, v.Message)
		}
		r.Fix("AR-001", report.Error, "artifact", msg, report.Remediation{
			Text:    "export it, define it in the .env next to the catalog, or give it a default in the artifact",
			Fixable: false,
		})
	}

	for _, f := range contract.EnvFiles {
		if !f.Required {
			continue
		}
		path := artifact.Interpolate(f.Path, o.Env)
		if _, err := os.Stat(path); err == nil {
			continue
		}
		r.Fix("AR-002", report.Error, "artifact/"+f.Service,
			fmt.Sprintf("the artifact loads %s, which does not exist", path),
			report.Remediation{
				Text:    "create it, or mark it optional in the artifact (env_file with required: false) so a missing file blocks only this service",
				Fixable: false,
			})
	}

	data.Artifact = rep
}
