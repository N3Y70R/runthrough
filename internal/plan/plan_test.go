// SPDX-License-Identifier: GPL-3.0-or-later

package plan_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/N3Y70R/runthrough/internal/catalog"
	"github.com/N3Y70R/runthrough/internal/plan"
	"github.com/N3Y70R/runthrough/internal/probe"
)

func build(t *testing.T) *plan.Plan {
	t.Helper()
	cat, err := catalog.Load(filepath.Join("testdata", "cat"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	p, err := plan.Build(context.Background(), cat, plan.Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return p
}

// The four branches of the readiness decision, which is where the worst bug
// of the third test round lived: a tcp probe against a published port was
// being reported as proof that a service was up.
func TestWaitTargetsPickTheHonestSignal(t *testing.T) {
	targets := map[string]probe.Target{}
	for _, tg := range build(t).WaitTargets(nil) {
		targets[tg.Service] = tg
	}

	if got := targets["with-healthcheck"].Kind; got != probe.KindContainer {
		t.Errorf("a service with a healthcheck in the artifact must use it, got %q", got)
	}
	if got := targets["from-image"].Kind; got != probe.KindHTTP {
		t.Errorf("http health with a published port must be probed over http, got %q", got)
	}
	if got := targets["builds-from-source"]; got.Kind != probe.KindUnverifiable {
		t.Errorf("tcp on a published port proves nothing and must be unverifiable, got %q", got.Kind)
	} else if got.Reason == "" {
		t.Error("an unverifiable target must explain what would make it verifiable")
	}
	if got := targets["no-published-port"]; got.Skip == "" {
		t.Errorf("a service with nothing published is out of reach and must be skipped, got %+v", got)
	}
}

// The catalog describes intent; the artifact decides what runs. rebuild used
// to claim it had rebuilt a service that only pulls an image.
func TestBuildableAsksTheArtifactNotTheCatalog(t *testing.T) {
	build, imageOnly := build(t).Buildable(nil)

	if !has(build, "builds-from-source") {
		t.Errorf("a service with a build section must be buildable, got %v", build)
	}
	if !has(imageOnly, "from-image") {
		t.Errorf("a service with only an image must not be reported as rebuilt, got %v", imageOnly)
	}
	if has(build, "from-image") {
		t.Error("the catalog says from-image has a repo, but the artifact says otherwise and wins")
	}
}

// Pinning one service to a branch must not move the others.
func TestWorktreeOverrideAppliesToOneService(t *testing.T) {
	cat, err := catalog.Load(filepath.Join("testdata", "cat"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := plan.Build(context.Background(), cat, plan.Options{
		Worktrees: map[string]string{"from-image": "feature/x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range p.Services {
		want := "main"
		if s.Name == "from-image" {
			want = "feature/x"
		}
		if s.Worktree != want {
			t.Errorf("%s: worktree %q, want %q", s.Name, s.Worktree, want)
		}
	}
}

// The env-file is what makes the printed command reproducible, so what it
// contains is part of the contract.
func TestEnvFileCarriesTheResolvedPlan(t *testing.T) {
	p := build(t)
	if p.Env["RT_PROJECT"] != p.Project {
		t.Errorf("the project name must reach the artifact, got %q", p.Env["RT_PROJECT"])
	}
	if p.Env["RT_FROM_IMAGE_HOST_PORT"] != "53001" {
		t.Errorf("host ports must be resolved, got %q", p.Env["RT_FROM_IMAGE_HOST_PORT"])
	}
	if _, ok := p.Env["RT_NO_PUBLISHED_PORT_HOST_PORT"]; ok {
		t.Error("a service that publishes nothing must not get a host port variable")
	}
}

func has(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
