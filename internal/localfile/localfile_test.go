// SPDX-License-Identifier: GPL-3.0-or-later

package localfile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/N3Y70R/runthrough/internal/catalog"
	"github.com/N3Y70R/runthrough/internal/localfile"
)

func setup(t *testing.T) (localfile.Context, string) {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	wt := filepath.Join(repo, "main")
	store := filepath.Join(repo, "artifacts")
	for _, dir := range []string{wt, store} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return localfile.Context{
		Store:        "{repo}/artifacts",
		Ecosystem:    "backend",
		Service:      "orders-api",
		RepoPath:     repo,
		WorktreePath: wt,
	}, store
}

func TestMissingSourceIsNotFixable(t *testing.T) {
	ctx, _ := setup(t)
	got := localfile.Resolve(ctx, catalog.LocalFile{From: ".env", To: "repo:.env", Mode: "link"})
	if got.Status != localfile.StatusMissingSource {
		t.Fatalf("status: got %s, want missing-source", got.Status)
	}
}

func TestMissingTargetWhenTheOriginalExists(t *testing.T) {
	ctx, store := setup(t)
	write(t, filepath.Join(store, ".env"), "TOKEN=x")
	got := localfile.Resolve(ctx, catalog.LocalFile{From: ".env", To: "worktree:.env", Mode: "link"})
	if got.Status != localfile.StatusMissingTarget {
		t.Fatalf("status: got %s, want missing-target", got.Status)
	}
	if filepath.Base(filepath.Dir(got.To)) != "main" {
		t.Errorf("the worktree anchor should resolve inside the worktree, got %s", got.To)
	}
}

func TestLinkToTheStoreIsOK(t *testing.T) {
	ctx, store := setup(t)
	src := filepath.Join(store, ".env")
	write(t, src, "TOKEN=x")
	if err := os.Symlink(src, filepath.Join(ctx.RepoPath, ".env")); err != nil {
		t.Fatal(err)
	}
	got := localfile.Resolve(ctx, catalog.LocalFile{From: ".env", To: "repo:.env", Mode: "link"})
	if got.Status != localfile.StatusOK {
		t.Fatalf("status: got %s, want ok", got.Status)
	}
}

func TestAnExistingFileIsNeverOverwritten(t *testing.T) {
	ctx, store := setup(t)
	write(t, filepath.Join(store, ".env"), "TOKEN=x")
	write(t, filepath.Join(ctx.RepoPath, ".env"), "someone wrote this by hand")
	got := localfile.Resolve(ctx, catalog.LocalFile{From: ".env", To: "repo:.env", Mode: "link"})
	if got.Status != localfile.StatusForeign {
		t.Fatalf("status: got %s, want foreign", got.Status)
	}
}

func TestALinkElsewhereIsReportedAsMismatch(t *testing.T) {
	ctx, store := setup(t)
	write(t, filepath.Join(store, ".env"), "TOKEN=x")
	other := filepath.Join(ctx.RepoPath, "other.env")
	write(t, other, "TOKEN=y")
	if err := os.Symlink(other, filepath.Join(ctx.RepoPath, ".env")); err != nil {
		t.Fatal(err)
	}
	got := localfile.Resolve(ctx, catalog.LocalFile{From: ".env", To: "repo:.env", Mode: "link"})
	if got.Status != localfile.StatusMismatch {
		t.Fatalf("status: got %s, want mismatch", got.Status)
	}
	if got.Detail == "" {
		t.Error("a mismatch should say where the link points")
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
