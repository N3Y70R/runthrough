// SPDX-License-Identifier: GPL-3.0-or-later

package doctor_test

import (
	"context"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/N3Y70R/runthrough/internal/catalog"
	"github.com/N3Y70R/runthrough/internal/doctor"
	"github.com/N3Y70R/runthrough/internal/report"
	"github.com/N3Y70R/runthrough/internal/runner"
)

// healthyDriver stands in for a working runtime, so that the checks under
// test are the catalog's and the artifact's, not the machine's.
type healthyDriver struct{}

func (healthyDriver) Name() string { return "fake" }

func (healthyDriver) Probe(context.Context) runner.Info {
	caps := map[string]bool{}
	for _, c := range runner.Required() {
		caps[string(c)] = true
	}
	return runner.Info{
		Driver: "fake", Available: true,
		Client: "25.0.0", Engine: "25.0.0", Compose: "2.24.0",
		Capabilities: caps,
	}
}

func (healthyDriver) Up(context.Context, runner.Invocation, runner.UpOptions, io.Writer, io.Writer) error {
	return nil
}
func (healthyDriver) Down(context.Context, runner.Invocation, runner.DownOptions, io.Writer, io.Writer) error {
	return nil
}
func (healthyDriver) Stop(context.Context, runner.Invocation, []string, io.Writer, io.Writer) error {
	return nil
}
func (healthyDriver) Build(context.Context, runner.Invocation, []string, io.Writer, io.Writer) error {
	return nil
}
func (healthyDriver) Logs(context.Context, runner.Invocation, []string, bool, io.Writer, io.Writer) error {
	return nil
}
func (healthyDriver) Status(context.Context, runner.Invocation) (map[string]runner.ServiceState, error) {
	return map[string]runner.ServiceState{}, nil
}

func load(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.Load(filepath.Join("testdata", "checks"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return c
}

func run(t *testing.T, services ...string) *report.Result {
	t.Helper()
	cat := load(t)
	return doctor.Run(context.Background(), doctor.Options{
		Catalog:  cat,
		Services: services,
		Artifact: cat.Resolve("compose/demo.yml"),
		Env:      map[string]string{"RT_PROJECT": "rt-demo"},
		Driver:   healthyDriver{},
	})
}

func codes(r *report.Result) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range r.Findings {
		if !seen[f.Code] {
			seen[f.Code] = true
			out = append(out, f.Code)
		}
	}
	sort.Strings(out)
	return out
}

// The golden set: every lesson the three test rounds taught, locked in one
// assertion. If a future change stops finding one of these, this fails here
// rather than in someone's afternoon.
func TestBrokenFixtureProducesTheExpectedFindings(t *testing.T) {
	got := codes(run(t))
	want := []string{
		"AR-001", // a variable the artifact interpolates with no default
		"AR-002", // a required env file that does not exist
		"AR-005", // a bind mount whose source is missing
		"EN-001", // a declared environment variable that is not defined
		"FS-001", // a declared file that does not exist
		"IN-001", // infrastructure the profile expects on this machine
		"RD-001", // a tcp probe on a published port, which cannot fail
		"WT-001", // the workspace does not exist
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("findings changed\n got: %v\nwant: %v", got, want)
	}
	if !r(t).HasErrors() {
		t.Error("a catalog this broken must fail")
	}
}

func r(t *testing.T) *report.Result { return run(t) }

// A requirement that names services must not block the others: the front of a
// stack should not need a secret only the backend uses.
func TestRequirementScopeExcludesOtherServices(t *testing.T) {
	withA := codes(run(t, "service-a"))
	withB := codes(run(t, "service-b"))

	if !contains(withA, "EN-001") {
		t.Errorf("service-a declares the requirement and must report it, got %v", withA)
	}
	if contains(withB, "EN-001") {
		t.Errorf("service-b does not need it and must not be blocked by it, got %v", withB)
	}
}

// Asking about one service means asking about what it stands on.
func TestScopeFollowsTheNeedsGraph(t *testing.T) {
	// service-a needs postgres, which the profile puts on this machine.
	if !contains(codes(run(t, "service-a")), "IN-001") {
		t.Error("the infrastructure a service needs must be checked with it")
	}
	// service-b needs nothing, so no infrastructure finding belongs to it.
	if contains(codes(run(t, "service-b")), "IN-001") {
		t.Error("a service that needs no infrastructure must not drag it in")
	}
}

// A readiness signal that cannot fail has to be called out before it is used
// to make a promise.
func TestTcpHealthOnAPublishedPortIsFlagged(t *testing.T) {
	got := run(t, "service-b")
	var found bool
	for _, f := range got.Findings {
		if f.Code == "RD-001" && strings.Contains(f.Scope, "service-b") {
			found = true
			if f.Remediation == nil {
				t.Error("RD-001 must say how to get a signal that means something")
			}
		}
	}
	if !found {
		t.Errorf("expected RD-001 for service-b, got %v", codes(got))
	}
}

// An unknown name must be rejected, and the message must say what the valid
// ones were: doctor is where people go when something does not add up.
func TestUnknownServiceListsTheValidOnes(t *testing.T) {
	got := run(t, "no-such-service")
	for _, f := range got.Findings {
		if f.Code != "CFG-001" {
			continue
		}
		if !strings.Contains(f.Message, "service-a") || !strings.Contains(f.Message, "service-b") {
			t.Errorf("the message must list the valid services, got %q", f.Message)
		}
		return
	}
	t.Errorf("expected CFG-001, got %v", codes(got))
}

// Every finding carries a remediation, so that a reader is never told what is
// wrong without being told what to do.
func TestEveryFindingCarriesARemediation(t *testing.T) {
	for _, f := range run(t).Findings {
		if f.Remediation == nil {
			t.Errorf("%s (%s) has no remediation: %s", f.Code, f.Scope, f.Message)
		}
	}
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
