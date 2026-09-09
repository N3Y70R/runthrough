// SPDX-License-Identifier: GPL-3.0-or-later

package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/N3Y70R/runthrough/internal/catalog"
	"github.com/N3Y70R/runthrough/internal/worktree"

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
func readContract(r *report.Result, o Options, data *Data) *artifact.Contract {
	if o.Artifact == "" {
		return nil
	}
	contract, err := artifact.Read(o.Artifact)
	if err != nil && contract == nil {
		r.Fix("AR-003", report.Error, "artifact", fmt.Sprintf("cannot read the driver artifact: %v", err),
			report.Remediation{Text: "check the compose file named by the ecosystem", Fixable: false})
		return nil
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

	// A bind mount whose source is missing does not fail: the runtime
	// creates an empty directory in its place, and the service starts with a
	// configuration that silently is not there.
	for name, svc := range contract.Services {
		for _, mount := range svc.Mounts {
			path := artifact.Interpolate(mount, o.Env)
			if strings.HasPrefix(path, "~") {
				path = worktree.ExpandHome(path)
			}
			if !filepath.IsAbs(path) {
				// The runtime resolves relative paths against the project
				// directory — the catalog — not against the artifact file.
				path = filepath.Join(o.Catalog.Dir, path)
			}
			if _, err := os.Stat(path); err == nil {
				continue
			}
			r.Fix("AR-005", report.Error, "artifact/"+name,
				fmt.Sprintf("the artifact mounts %s, which does not exist", path),
				report.Remediation{
					Text:    "create it — the runtime would create an empty directory in its place and the service would start misconfigured",
					Fixable: false,
				})
		}
	}

	data.Artifact = rep
	return contract
}

// checkReadiness warns when a service declares a readiness signal that cannot
// actually fail. A tcp probe against a port the runtime publishes succeeds
// before the service is listening, so a green result would prove nothing —
// and a false promise is worse than no promise.
func checkReadiness(r *report.Result, contract *artifact.Contract, eco, name string, svc *catalog.Service) {
	if svc.Port.Host == 0 || svc.Health.Type != "tcp" {
		return
	}
	if contract != nil {
		if declared, ok := contract.Service(name); ok && declared.Healthcheck {
			return
		}
	}
	r.Fix("RD-001", report.Warning, eco+"/"+name,
		fmt.Sprintf("health type tcp on published port %d cannot fail: the runtime answers it before the service does", svc.Port.Host),
		report.Remediation{
			Text:    "declare health.type http with a path the service serves, or add a healthcheck to the service in the artifact",
			Fixable: false,
		})
}
