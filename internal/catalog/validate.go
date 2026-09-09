// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/N3Y70R/runthrough/internal/report"
)

// SupportedVersion is the catalog schema this build understands.
const SupportedVersion = 1

// Validate checks the catalog for anything that would make a later command
// fail halfway. Findings carry stable codes so agents and scripts can branch
// on them (T-10); the wording may change, the code may not.
//
// Errors are things that cannot work. Warnings are things that are probably a
// mistake but that the tool can still run with — an unknown runtime, a port
// outside the declared range — because a soft vocabulary must not block a
// catalog just for being newer than this binary.
func (c *Catalog) Validate() []report.Finding {
	var f findings

	if c.Version != SupportedVersion {
		f.errorf("CAT-001", "", "catalog version %d is not supported (this build understands version %d)", c.Version, SupportedVersion)
	}
	if len(c.Ecosystems) == 0 {
		f.errorf("CAT-002", "", "the catalog declares no ecosystems")
	}
	if mode := c.Defaults.LocalFiles.Mode; !oneOf(mode, KnownModes) {
		f.errorf("CAT-003", "defaults", "local file mode %q is not one of %s", mode, strings.Join(KnownModes, ", "))
	}

	validateRequires(&f, "catalog", c.Requires)

	infra := c.Infra.Components()
	hostPorts := map[int]string{}

	for _, ecoName := range sortedKeys(c.Ecosystems) {
		eco := c.Ecosystems[ecoName]
		c.validateEcosystem(&f, eco, infra, hostPorts)
	}
	return f
}

func (c *Catalog) validateEcosystem(f *findings, eco *Ecosystem, infra map[string]bool, hostPorts map[int]string) {
	if eco.Workspace == "" {
		f.errorf("CAT-004", eco.Name, "the ecosystem declares no workspace")
	}
	if eco.DefaultWorktree == "" {
		f.warnf("CAT-005", eco.Name, "no default_worktree: every service will have to name its own")
	}
	if eco.Compose != "" {
		if _, err := os.Stat(c.Resolve(eco.Compose)); err != nil {
			f.errorf("CAT-006", eco.Name, "compose file %q does not exist relative to the manifest", eco.Compose)
		}
	}
	if len(eco.Services) == 0 {
		f.warnf("CAT-007", eco.Name, "the ecosystem declares no services")
	}

	roots := 0
	for _, svcName := range sortedKeys(eco.Services) {
		svc := eco.Services[svcName]
		scope := eco.Name + "/" + svcName

		if svc.Port.Internal == 0 && svc.Dev == nil {
			f.errorf("CAT-008", scope, "no internal port declared")
		}
		if svc.Port.Host != 0 {
			if owner, taken := hostPorts[svc.Port.Host]; taken {
				f.errorf("CAT-009", scope, "host port %d is already published by %s", svc.Port.Host, owner)
			} else {
				hostPorts[svc.Port.Host] = scope
			}
			if len(eco.PortRange) == 2 && (svc.Port.Host < eco.PortRange[0] || svc.Port.Host > eco.PortRange[1]) {
				f.warnf("CAT-010", scope, "host port %d falls outside the ecosystem range %d-%d", svc.Port.Host, eco.PortRange[0], eco.PortRange[1])
			}
		}
		if svc.Runtime == "" && svc.Image == "" {
			f.errorf("CAT-011", scope, "no runtime declared: a service either builds from code or names an image")
		} else if svc.Runtime != "" && !oneOf(svc.Runtime, KnownRuntimes) {
			f.warnf("CAT-012", scope, "runtime %q is not one this build knows about", svc.Runtime)
		}

		switch svc.Health.Type {
		case "":
			f.warnf("CAT-013", scope, "no healthcheck declared: start-up order cannot be guaranteed for this service")
		case "http":
			if svc.Health.Path == "" {
				f.errorf("CAT-014", scope, "an http healthcheck needs a path")
			}
		case "command":
			if svc.Health.Command == "" {
				f.errorf("CAT-014", scope, "a command healthcheck needs a command")
			}
		case "tcp":
		default:
			f.errorf("CAT-015", scope, "healthcheck type %q is not one of %s", svc.Health.Type, strings.Join(KnownHealthTypes, ", "))
		}

		for _, need := range svc.Needs {
			if _, isService := eco.Services[need]; isService {
				continue
			}
			if infra[need] {
				continue
			}
			f.errorf("CAT-016", scope, "needs %q, which is neither a service of this ecosystem nor a declared infrastructure component", need)
		}

		for i, lf := range svc.LocalFiles {
			if lf.From == "" {
				f.errorf("CAT-017", scope, "local file %d has no source", i+1)
			}
			if !hasAnchor(lf.To) {
				f.errorf("CAT-018", scope, "local file target %q must start with an anchor (%s)", lf.To, strings.Join(LocalFileAnchors, " or "))
			}
			if !oneOf(lf.Mode, KnownModes) {
				f.errorf("CAT-019", scope, "local file mode %q is not one of %s", lf.Mode, strings.Join(KnownModes, ", "))
			}
		}

		validateRequires(f, scope, svc.Requires)

		if svc.GatewayRoot {
			roots++
		}
	}

	if roots > 1 {
		f.errorf("CAT-020", eco.Name, "%d services claim gateway_root; only one can serve the root", roots)
	}
	if eco.Gateway != nil {
		for _, prefix := range sortedKeys(eco.Gateway.Routes) {
			target := eco.Gateway.Routes[prefix]
			if _, ok := eco.Services[target]; !ok {
				f.errorf("CAT-021", eco.Name, "gateway route %q points at %q, which is not a service of this ecosystem", prefix, target)
			}
		}
		if eco.Gateway.Port != 0 {
			if owner, taken := hostPorts[eco.Gateway.Port]; taken {
				f.errorf("CAT-009", eco.Name, "gateway port %d is already published by %s", eco.Gateway.Port, owner)
			} else {
				hostPorts[eco.Gateway.Port] = eco.Name + "/gateway"
			}
		}
	}

	c.validateCycles(f, eco)
}

// validateCycles reports dependency cycles, which would deadlock start-up
// ordering (H-02).
func (c *Catalog) validateCycles(f *findings, eco *Ecosystem) {
	const (
		white = 0
		grey  = 1
		black = 2
	)
	state := map[string]int{}
	var stack []string
	var visit func(name string) bool

	visit = func(name string) bool {
		state[name] = grey
		stack = append(stack, name)
		svc := eco.Services[name]
		if svc != nil {
			for _, need := range svc.Needs {
				if _, ok := eco.Services[need]; !ok {
					continue
				}
				switch state[need] {
				case grey:
					f.errorf("CAT-022", eco.Name, "dependency cycle: %s -> %s", strings.Join(append(stack, need), " -> "), need)
					return true
				case white:
					if visit(need) {
						return true
					}
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[name] = black
		return false
	}

	for _, name := range sortedKeys(eco.Services) {
		if state[name] == white {
			if visit(name) {
				return
			}
		}
	}
}

func validateRequires(f *findings, scope string, req Requires) {
	for i, e := range req.Env {
		if e.Name == "" {
			f.errorf("CAT-023", scope, "required env %d has no name", i+1)
		}
		if !oneOf(e.Phase, KnownPhases) {
			f.errorf("CAT-024", scope, "required env %q declares phase %q, which is not build or run", e.Name, e.Phase)
		}
	}
	for i, file := range req.Files {
		if file.Path == "" {
			f.errorf("CAT-025", scope, "required file %d has no path", i+1)
		}
	}
}

type findings []report.Finding

func (f *findings) errorf(code, scope, format string, args ...any) {
	*f = append(*f, report.Finding{Code: code, Severity: report.Error, Scope: scope, Message: fmt.Sprintf(format, args...)})
}

func (f *findings) warnf(code, scope, format string, args ...any) {
	*f = append(*f, report.Finding{Code: code, Severity: report.Warning, Scope: scope, Message: fmt.Sprintf(format, args...)})
}

func oneOf(v string, set []string) bool {
	for _, s := range set {
		if v == s {
			return true
		}
	}
	return false
}

func hasAnchor(target string) bool {
	for _, a := range LocalFileAnchors {
		if strings.HasPrefix(target, a) && len(target) > len(a) {
			return true
		}
	}
	return false
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
