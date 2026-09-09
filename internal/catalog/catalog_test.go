// SPDX-License-Identifier: GPL-3.0-or-later

package catalog_test

import (
	"path/filepath"
	"testing"

	"github.com/N3Y70R/runthrough/internal/catalog"
	"github.com/N3Y70R/runthrough/internal/report"
)

// The example catalog doubles as documentation, so it must stay clean.
func TestExampleCatalogValidatesClean(t *testing.T) {
	c, err := catalog.Load(filepath.Join("..", "..", "examples", "catalog"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.Path == "" || c.Dir == "" {
		t.Fatalf("manifest path and dir must be recorded, got %q / %q", c.Path, c.Dir)
	}
	for _, f := range c.Validate() {
		if f.Severity == report.Error {
			t.Errorf("unexpected error %s (%s): %s", f.Code, f.Scope, f.Message)
		}
	}
}

func TestDefaultsAreApplied(t *testing.T) {
	c, err := catalog.Load(filepath.Join("..", "..", "examples", "catalog"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	eco := c.Ecosystems["backend"]
	if eco == nil {
		t.Fatal("ecosystem backend missing")
	}
	if eco.Name != "backend" {
		t.Errorf("ecosystem name not filled in: %q", eco.Name)
	}
	svc := eco.Services["orders-api"]
	if svc.Name != "orders-api" {
		t.Errorf("service name not filled in: %q", svc.Name)
	}
	// repo defaults to the service name, worktree to the ecosystem default.
	if svc.Repo != "orders-api" {
		t.Errorf("repo default: got %q", svc.Repo)
	}
	if got := eco.WorktreeOf(svc); got != "main" {
		t.Errorf("worktree default: got %q, want main", got)
	}
	if svc.LocalFiles[0].Mode != "link" {
		t.Errorf("local file mode default: got %q", svc.LocalFiles[0].Mode)
	}
}

func TestValidateCatchesBrokenCatalog(t *testing.T) {
	c, err := catalog.Load(filepath.Join("testdata", "broken"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	got := map[string]bool{}
	for _, f := range c.Validate() {
		got[f.Code] = true
	}
	want := []string{
		"CAT-009", // duplicate host port
		"CAT-012", // unknown runtime (warning)
		"CAT-014", // http health without a path
		"CAT-015", // unknown health type
		"CAT-016", // needs an unknown target
		"CAT-018", // local file target without an anchor
		"CAT-022", // dependency cycle
	}
	for _, code := range want {
		if !got[code] {
			t.Errorf("expected finding %s, got %v", code, keys(got))
		}
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
