// SPDX-License-Identifier: GPL-3.0-or-later

package doctor

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/N3Y70R/runthrough/internal/catalog"
	"github.com/N3Y70R/runthrough/internal/report"
	"github.com/N3Y70R/runthrough/internal/worktree"
)

// RequirementReport is one declared requirement and where it was satisfied.
// Only presence is ever recorded — never a value, not even truncated.
type RequirementReport struct {
	Kind  string `json:"kind"` // env or file
	Name  string `json:"name"`
	Scope string `json:"scope"`
	// AppliesTo names the services a catalog-level requirement belongs to.
	// Without it, "scope: catalog" on a requirement that no longer applies
	// to the whole catalog cannot be explained without opening the yaml.
	AppliesTo []string `json:"applies_to,omitempty"`
	Present   bool     `json:"present"`
	Source    string   `json:"source,omitempty"`
}

// envSource says where a variable is defined, without reading its value.
type envSource struct {
	// catalogEnv holds the names defined in the catalog's own .env.
	catalogEnv map[string]bool
	catalogDir string
}

func newEnvSource(dir string) envSource {
	return envSource{catalogEnv: envNames(filepath.Join(dir, ".env")), catalogDir: dir}
}

// find reports where a variable comes from, or an empty string.
func (e envSource) find(name string) string {
	if _, ok := os.LookupEnv(name); ok {
		return "the environment"
	}
	if e.catalogEnv[name] {
		return ".env next to the catalog"
	}
	return ""
}

// envNames reads only the KEYS of an env file. The values are none of the
// tool's business, and never leave this function.
func envNames(path string) map[string]bool {
	out := map[string]bool{}
	file, err := os.Open(path)
	if err != nil {
		return out
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		if key, _, ok := strings.Cut(line, "="); ok {
			if key = strings.TrimSpace(key); key != "" {
				out[key] = true
			}
		}
	}
	return out
}

// checkRequires verifies that what a catalog or service declares it needs is
// present. It reports what is missing and why it matters; it never invents a
// value, and never prints one.
func checkRequires(r *report.Result, src envSource, scope string, req catalog.Requires, paths map[string]string, data *Data, inScope []string) {
	for _, e := range req.Env {
		if e.Name == "" || !e.Applies(inScope) {
			continue
		}
		source := src.find(e.Name)
		data.Requirements = append(data.Requirements, RequirementReport{
			Kind: "env", Name: e.Name, Scope: scope, AppliesTo: e.Services,
			Present: source != "", Source: source,
		})
		if source != "" {
			continue
		}
		severity := report.Error
		code := "EN-001"
		if e.Optional {
			severity = report.Warning
			code = "EN-002"
		}
		msg := fmt.Sprintf("%s is not defined", e.Name)
		if e.Phase != "" {
			msg += fmt.Sprintf(" (needed at %s time)", e.Phase)
		}
		if e.Why != "" {
			msg += ": " + e.Why
		}
		// The two paths are not equivalent, and saying "or" as if they were
		// let a tester believe the problem was solved: exporting satisfies
		// this run only, while the .env travels with the catalog and reaches
		// the command the tool prints for you to paste.
		r.Fix(code, severity, scope, msg, report.Remediation{
			Text:    fmt.Sprintf("define %s in the .env next to the catalog so it travels with it; exporting it in your shell also works, but only for commands run from that shell. Either way the tool checks that it exists, never what it holds", e.Name),
			Fixable: false,
		})
	}

	for _, f := range req.Files {
		if !f.Applies(inScope) {
			continue
		}
		path := expandPath(f.Path, src.catalogDir, paths)
		_, err := os.Stat(path)
		present := err == nil
		data.Requirements = append(data.Requirements, RequirementReport{
			Kind: "file", Name: path, Scope: scope, AppliesTo: f.Services, Present: present,
		})
		if present {
			continue
		}
		severity := report.Error
		code := "FS-001"
		if f.Optional {
			severity = report.Warning
			code = "FS-002"
		}
		msg := fmt.Sprintf("%s does not exist", path)
		if f.Why != "" {
			msg += ": " + f.Why
		}
		r.Fix(code, severity, scope, msg, report.Remediation{
			Text:    "create it, or ask whoever holds it — if it carries secrets, the tool will not generate it",
			Fixable: false,
		})
	}
}

func expandPath(path, catalogDir string, paths map[string]string) string {
	replacements := []string{"{catalog}", catalogDir}
	for k, v := range paths {
		replacements = append(replacements, "{"+k+"}", v)
	}
	return worktree.ExpandHome(strings.NewReplacer(replacements...).Replace(path))
}
