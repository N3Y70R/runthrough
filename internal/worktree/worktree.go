// SPDX-License-Identifier: GPL-3.0-or-later

// Package worktree locates the code a service is built from.
//
// It reads the layout on disk rather than depending on any particular tool,
// so a repository managed with worktrees and a plain clone both work (G-03).
// What it reports — branch and commit per service — is the seed of
// traceability: without it, an observed effect cannot be attributed to a
// change (G-05).
package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Layout is how a repository is arranged on disk.
type Layout string

const (
	// LayoutWorktree is a bare repository with one directory per branch.
	LayoutWorktree Layout = "worktree"
	// LayoutPlain is an ordinary clone with a single checkout.
	LayoutPlain Layout = "plain"
	// LayoutMissing is a repository that is not there.
	LayoutMissing Layout = "missing"
)

// Location is where a service's code lives and what state it is in.
type Location struct {
	Repo     string `json:"repo_path"`
	Path     string `json:"path"`
	Layout   Layout `json:"layout"`
	Exists   bool   `json:"exists"`
	Branch   string `json:"branch,omitempty"`
	Commit   string `json:"commit,omitempty"`
	Dirty    bool   `json:"dirty,omitempty"`
	GitError string `json:"git_error,omitempty"`
}

// Resolve locates the checkout a service builds from: workspace, repository
// and the requested worktree.
func Resolve(ctx context.Context, workspace, repo, want string) Location {
	repoPath := filepath.Join(ExpandHome(workspace), repo)
	loc := Location{Repo: repoPath, Layout: LayoutMissing}

	info, err := os.Stat(repoPath)
	if err != nil || !info.IsDir() {
		loc.Path = filepath.Join(repoPath, want)
		return loc
	}

	// A bare repository with per-branch directories: the checkout is the
	// worktree directory.
	if isDir(filepath.Join(repoPath, ".bare")) {
		loc.Layout = LayoutWorktree
		loc.Path = filepath.Join(repoPath, want)
		loc.Exists = isDir(loc.Path)
	} else if exists(filepath.Join(repoPath, ".git")) {
		loc.Layout = LayoutPlain
		loc.Path = repoPath
		loc.Exists = true
	} else {
		// A directory that is not a repository at all.
		loc.Path = filepath.Join(repoPath, want)
		return loc
	}

	if loc.Exists {
		loc.readGit(ctx)
	}
	return loc
}

func (l *Location) readGit(ctx context.Context) {
	if branch, err := git(ctx, l.Path, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		l.Branch = branch
	} else {
		l.GitError = err.Error()
		return
	}
	if commit, err := git(ctx, l.Path, "rev-parse", "--short", "HEAD"); err == nil {
		l.Commit = commit
	}
	if status, err := git(ctx, l.Path, "status", "--porcelain"); err == nil {
		l.Dirty = strings.TrimSpace(status) != ""
	}
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ExpandHome turns a leading ~ into the user's home directory.
func ExpandHome(p string) string {
	if p == "" || p[0] != '~' {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	if len(p) > 1 && (p[1] == '/' || p[1] == filepath.Separator) {
		return filepath.Join(home, p[2:])
	}
	return p
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
