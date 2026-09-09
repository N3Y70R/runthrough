// SPDX-License-Identifier: GPL-3.0-or-later

// Package config resolves which catalog a command works with, and reads the
// per-user settings.
//
// Answering "which catalog am I using, and why that one?" is the first thing
// anyone needs when something does not add up, so the resolution reports both
// the path and the way it was found (D-9, T-09).
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/N3Y70R/runthrough/internal/catalog"
)

// EnvCatalog overrides the catalog without a flag.
const EnvCatalog = "RUNTHROUGH_CATALOG"

// Source is how a catalog path was arrived at.
type Source string

const (
	SourceFlag Source = "flag"
	SourceEnv  Source = "environment"
	SourceScan Source = "directory scan"
	SourceUser Source = "user config"
	SourceNone Source = "none"
)

// User is the per-user configuration file.
type User struct {
	DefaultCatalog string `yaml:"default_catalog"`
	SnapshotsDir   string `yaml:"snapshots_dir"`

	Path   string `yaml:"-"`
	Exists bool   `yaml:"-"`
}

// UserPath is where the configuration lives, honouring XDG_CONFIG_HOME.
func UserPath() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "runthrough", "config.yaml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "runthrough", "config.yaml")
	}
	return filepath.Join(home, ".config", "runthrough", "config.yaml")
}

// LoadUser reads the user configuration. A missing file is not an error: it
// means every default applies.
func LoadUser() (*User, error) {
	path := UserPath()
	u := &User{Path: path}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return u, nil
	}
	if err != nil {
		return u, err
	}
	if err := yaml.Unmarshal(data, u); err != nil {
		return u, fmt.Errorf("%s: %w", path, err)
	}
	u.Path = path
	u.Exists = true
	return u, nil
}

// SnapshotsRoot is where captures are kept: outside git, in a configurable
// path, defaulting to the user's data directory (DA-06).
func (u *User) SnapshotsRoot() string {
	if u != nil && u.SnapshotsDir != "" {
		return u.SnapshotsDir
	}
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "runthrough", "snapshots")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".local", "share", "runthrough", "snapshots")
	}
	return filepath.Join(home, ".local", "share", "runthrough", "snapshots")
}

// Resolution is the outcome of the lookup: which catalog, and how it was found.
type Resolution struct {
	Path        string `json:"path"`
	Source      Source `json:"source"`
	ScannedFrom string `json:"scanned_from,omitempty"`
}

// Lookup is what the resolution chain consults, in order.
type Lookup struct {
	Flag string
	Env  string
	Cwd  string
	User *User
}

// ResolveCatalog walks the chain and returns the first hit: explicit flag,
// environment variable, a manifest found by scanning upwards from the working
// directory, then the catalog registered in the user configuration.
func ResolveCatalog(l Lookup) (Resolution, error) {
	if l.Flag != "" {
		return Resolution{Path: l.Flag, Source: SourceFlag}, nil
	}
	if l.Env != "" {
		return Resolution{Path: l.Env, Source: SourceEnv}, nil
	}
	if l.Cwd != "" {
		if found, ok := scanUp(l.Cwd); ok {
			return Resolution{Path: found, Source: SourceScan, ScannedFrom: l.Cwd}, nil
		}
	}
	if l.User != nil && l.User.DefaultCatalog != "" {
		return Resolution{Path: l.User.DefaultCatalog, Source: SourceUser}, nil
	}
	return Resolution{Source: SourceNone}, fmt.Errorf("no catalog found: pass --catalog, set %s, run inside a catalog, or register default_catalog in %s", EnvCatalog, UserPath())
}

// scanUp looks for a manifest in dir and every parent up to the root.
func scanUp(dir string) (string, bool) {
	current, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for {
		candidate := filepath.Join(current, catalog.ManifestName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		current = parent
	}
}
