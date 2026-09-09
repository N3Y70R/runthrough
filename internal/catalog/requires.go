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
}

// FileRequirement is one file that must exist. Placeholders {catalog},
// {repo} and {worktree} are resolved before checking.
type FileRequirement struct {
	Path     string `yaml:"path"`
	Why      string `yaml:"why"`
	Optional bool   `yaml:"optional"`
}

// KnownPhases are the values Phase accepts.
var KnownPhases = []string{"", "build", "run"}
