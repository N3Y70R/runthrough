// SPDX-License-Identifier: GPL-3.0-or-later

// Package localfile resolves the unversioned files a service needs.
//
// The original lives in a store outside the versioned tree; the tool places
// it where that service looks for it, which is not the same place for every
// service (S-02, D-10). This package only resolves and inspects: it never
// writes. Writing belongs to `doctor --fix`, under rules that are not
// configurable.
package localfile

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/N3Y70R/runthrough/internal/catalog"
	"github.com/N3Y70R/runthrough/internal/worktree"
)

// Status is the state of one declared file.
type Status string

const (
	// StatusOK means the file is where the service expects it.
	StatusOK Status = "ok"
	// StatusMissingSource means the original is absent from the store: the
	// tool cannot invent it, because that would mean inventing secrets.
	StatusMissingSource Status = "missing-source"
	// StatusMissingTarget means the original exists and only the placement
	// is missing. This is the one case --fix can resolve.
	StatusMissingTarget Status = "missing-target"
	// StatusForeign means something else already occupies the target. The
	// tool never overwrites.
	StatusForeign Status = "foreign"
	// StatusMismatch means the target is a link pointing somewhere else.
	StatusMismatch Status = "mismatch"
)

// Plan is one resolved declaration.
type Plan struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Mode     string `json:"mode"`
	Required bool   `json:"required"`
	Status   Status `json:"status"`
	Detail   string `json:"detail,omitempty"`
}

// Context is what placeholders resolve against.
type Context struct {
	Store        string
	Ecosystem    string
	Service      string
	RepoPath     string
	WorktreePath string
}

// Resolve turns a declaration into absolute paths and inspects the result.
func Resolve(ctx Context, f catalog.LocalFile) Plan {
	store := expand(ctx.Store, ctx)
	p := Plan{
		From:     filepath.Join(store, f.From),
		Mode:     f.Mode,
		Required: f.Required,
	}
	p.To = target(ctx, f.To)
	p.Status = inspect(p)
	if p.Status == StatusMismatch {
		if dest, err := os.Readlink(p.To); err == nil {
			p.Detail = "points at " + dest
		}
	}
	return p
}

// target resolves the anchored destination. An unanchored target is rejected
// by validation (CAT-018) long before it gets here.
func target(ctx Context, to string) string {
	switch {
	case strings.HasPrefix(to, "repo:"):
		return filepath.Join(ctx.RepoPath, strings.TrimPrefix(to, "repo:"))
	case strings.HasPrefix(to, "worktree:"):
		return filepath.Join(ctx.WorktreePath, strings.TrimPrefix(to, "worktree:"))
	default:
		return to
	}
}

func inspect(p Plan) Status {
	if _, err := os.Stat(p.From); err != nil {
		return StatusMissingSource
	}
	info, err := os.Lstat(p.To)
	if err != nil {
		return StatusMissingTarget
	}
	if info.Mode()&os.ModeSymlink == 0 {
		if p.Mode == "copy" {
			return StatusOK
		}
		return StatusForeign
	}
	dest, err := os.Readlink(p.To)
	if err != nil {
		return StatusMismatch
	}
	if !filepath.IsAbs(dest) {
		dest = filepath.Join(filepath.Dir(p.To), dest)
	}
	if sameFile(dest, p.From) {
		return StatusOK
	}
	return StatusMismatch
}

func sameFile(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(ai, bi)
}

func expand(s string, ctx Context) string {
	r := strings.NewReplacer(
		"{repo}", ctx.RepoPath,
		"{worktree}", ctx.WorktreePath,
		"{service}", ctx.Service,
		"{eco}", ctx.Ecosystem,
	)
	return worktree.ExpandHome(r.Replace(s))
}
