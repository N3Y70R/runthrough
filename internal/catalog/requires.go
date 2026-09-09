// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

// Requires is what a catalog or a service needs present before it can work:
// environment variables and files.
//
// The tool checks PRESENCE and never content. A missing secret has to be
// reported — a stack that starts without it fails later and confusingly — but
// reading, printing or guessing its value is not the tool's business.
type Requires struct {
	Env   []EnvRequirement  `yaml:"env"`
	Files []FileRequirement `yaml:"files"`
}

// EnvRequirement is one variable that must be defined somewhere.
type EnvRequirement struct {
	Name string `yaml:"name"`
	// Why is shown in the diagnosis, so that whoever hits it knows what to
	// ask for rather than only what is missing.
	Why      string `yaml:"why"`
	Optional bool   `yaml:"optional"`
	// Phase narrows when it matters: "build", "run", or empty for both.
	Phase string `yaml:"phase"`
	// Services narrows WHO needs it. A secret shared by three services does
	// not belong to the fourth, and blocking that one on it undoes the
	// point of checking a slice of the stack at a time.
	Services []string `yaml:"services"`
}

// Applies reports whether a requirement is in scope. A requirement that names
// no services applies to the whole catalog.
func (e EnvRequirement) Applies(scope []string) bool {
	return appliesTo(e.Services, scope)
}

// Applies reports whether a file requirement is in scope.
func (f FileRequirement) Applies(scope []string) bool {
	return appliesTo(f.Services, scope)
}

func appliesTo(declared, scope []string) bool {
	if len(declared) == 0 || len(scope) == 0 {
		return true
	}
	for _, d := range declared {
		for _, s := range scope {
			if d == s {
				return true
			}
		}
	}
	return false
}

// FileRequirement is one file that must exist. Placeholders {catalog},
// {repo} and {worktree} are resolved before checking.
type FileRequirement struct {
	Path     string   `yaml:"path"`
	Why      string   `yaml:"why"`
	Optional bool     `yaml:"optional"`
	Services []string `yaml:"services"`
}

// KnownPhases are the values Phase accepts.
var KnownPhases = []string{"", "build", "run"}
