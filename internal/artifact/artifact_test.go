// SPDX-License-Identifier: GPL-3.0-or-later

package artifact_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/N3Y70R/runthrough/internal/artifact"
)

const sample = `
name: ${RT_PROJECT}
services:
  api:
    image: ${RT_API_IMAGE:-node:22-alpine}
    build:
      context: ${RT_API_CONTEXT}
      args:
        TOKEN: "${REGISTRY_TOKEN:-}"
        SECRET: "${MUST_EXIST:?the build needs it}"
    env_file:
      - path: ${RT_API_CONTEXT}/.env
        required: false
      - ${RT_API_CONTEXT}/.env.shared
    command: echo "$$NOT_A_VARIABLE and $PLAIN_ONE"
  web:
    image: nginx
    env_file: ./shared.env
`

func read(t *testing.T) *artifact.Contract {
	t.Helper()
	path := filepath.Join(t.TempDir(), "compose.yml")
	if err := os.WriteFile(path, []byte(sample), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := artifact.Read(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return c
}

func TestVariablesDistinguishDefaultsFromRequirements(t *testing.T) {
	c := read(t)
	got := map[string]artifact.Variable{}
	for _, v := range c.Variables {
		got[v.Name] = v
	}

	if _, ok := got["NOT_A_VARIABLE"]; ok {
		t.Error("$$ is an escaped dollar sign, not an interpolation")
	}
	if !got["RT_API_IMAGE"].HasDefault {
		t.Error("${VAR:-default} must count as having a default")
	}
	if got["RT_PROJECT"].HasDefault {
		t.Error("${VAR} has no default")
	}
	if !got["PLAIN_ONE"].HasDefault && got["PLAIN_ONE"].Name != "PLAIN_ONE" {
		t.Error("$VAR without braces must be found")
	}
	if v := got["MUST_EXIST"]; !v.Explicit || v.Message != "the build needs it" {
		t.Errorf("${VAR:?message} must carry its own message, got %+v", v)
	}
	// REGISTRY_TOKEN uses ${VAR:-} — an empty default is still a default.
	if !got["REGISTRY_TOKEN"].HasDefault {
		t.Error("an empty default is still a default")
	}
}

func TestMissingIgnoresWhatIsResolved(t *testing.T) {
	c := read(t)
	resolved := map[string]bool{"RT_PROJECT": true, "RT_API_CONTEXT": true, "PLAIN_ONE": true}

	missing := c.Missing(func(name string) bool { return resolved[name] })
	if len(missing) != 1 || missing[0].Name != "MUST_EXIST" {
		t.Fatalf("expected only MUST_EXIST to be missing, got %+v", missing)
	}
}

func TestEnvFilesCoverEveryShape(t *testing.T) {
	c := read(t)
	if len(c.EnvFiles) != 3 {
		t.Fatalf("expected three env file references, got %d: %+v", len(c.EnvFiles), c.EnvFiles)
	}
	var optional, required int
	for _, f := range c.EnvFiles {
		if f.Required {
			required++
		} else {
			optional++
		}
	}
	if optional != 1 || required != 2 {
		t.Errorf("required: false must be honoured; got %d optional, %d required", optional, required)
	}
}

func TestInterpolateLeavesUnknownNamesAlone(t *testing.T) {
	values := map[string]string{"RT_API_CONTEXT": "/work/api"}
	if got := artifact.Interpolate("${RT_API_CONTEXT}/.env", values); got != "/work/api/.env" {
		t.Errorf("got %q", got)
	}
	if got := artifact.Interpolate("${UNKNOWN}/x", values); got != "${UNKNOWN}/x" {
		t.Errorf("an unresolved name must stay visible, got %q", got)
	}
}
