// SPDX-License-Identifier: GPL-3.0-or-later

package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/N3Y70R/runthrough/internal/config"
)

func TestResolutionChainPrefersTheFlag(t *testing.T) {
	got, err := config.ResolveCatalog(config.Lookup{
		Flag: "/from/flag",
		Env:  "/from/env",
		User: &config.User{DefaultCatalog: "/from/user"},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.Path != "/from/flag" || got.Source != config.SourceFlag {
		t.Errorf("got %+v, want the flag to win", got)
	}
}

func TestResolutionChainFallsBackInOrder(t *testing.T) {
	user := &config.User{DefaultCatalog: "/from/user"}

	env, err := config.ResolveCatalog(config.Lookup{Env: "/from/env", User: user})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if env.Source != config.SourceEnv {
		t.Errorf("environment should beat the user config, got %s", env.Source)
	}

	fallback, err := config.ResolveCatalog(config.Lookup{User: user})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if fallback.Source != config.SourceUser || fallback.Path != "/from/user" {
		t.Errorf("got %+v, want the user config", fallback)
	}
}

func TestResolutionScansUpwards(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, "runthrough.yaml")
	if err := os.WriteFile(manifest, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := config.ResolveCatalog(config.Lookup{Cwd: deep})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.Source != config.SourceScan {
		t.Fatalf("source: got %s, want a directory scan", got.Source)
	}
	if got.Path != manifest {
		t.Errorf("path: got %s, want %s", got.Path, manifest)
	}
	if got.ScannedFrom != deep {
		t.Errorf("scanned_from: got %s, want %s", got.ScannedFrom, deep)
	}
}

func TestNoCatalogIsAnActionableError(t *testing.T) {
	_, err := config.ResolveCatalog(config.Lookup{Cwd: t.TempDir(), User: &config.User{}})
	if err == nil {
		t.Fatal("expected an error when nothing resolves")
	}
	if len(err.Error()) < 40 {
		t.Errorf("the error should say how to fix it, got %q", err)
	}
}
