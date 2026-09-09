// SPDX-License-Identifier: GPL-3.0-or-later

package report_test

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/N3Y70R/runthrough/internal/report"
)

// The severity of a finding decides the exit code, and the third test round
// showed why it matters: a service that never started was a warning, so a
// pipeline step passed green.
func TestExitCodeFollowsSeverity(t *testing.T) {
	cases := []struct {
		name     string
		severity report.Severity
		status   string
		exit     int
	}{
		{"nothing", "", "ok", 0},
		{"info", report.Info, "ok", 0},
		{"warning", report.Warning, "warning", 0},
		{"error", report.Error, "error", 1},
	}
	for _, c := range cases {
		r := report.New("test")
		if c.severity != "" {
			r.Addf("X-001", c.severity, "scope", "something")
		}
		if r.Status != c.status {
			t.Errorf("%s: status %q, want %q", c.name, r.Status, c.status)
		}
		if r.ExitCode() != c.exit {
			t.Errorf("%s: exit %d, want %d", c.name, r.ExitCode(), c.exit)
		}
	}
}

// An error must not be downgraded by a warning that arrives after it.
func TestErrorSurvivesLaterWarnings(t *testing.T) {
	r := report.New("test")
	r.Addf("X-001", report.Error, "", "broken")
	r.Addf("X-002", report.Warning, "", "odd")
	if r.Status != "error" || r.ExitCode() != 1 {
		t.Errorf("status %q, exit %d", r.Status, r.ExitCode())
	}
}

// The JSON is the primary output: an agent branches on the code, and reads
// whether a finding can be fixed automatically.
func TestJSONKeepsCodesAndRemediation(t *testing.T) {
	r := report.New("doctor")
	r.Fix("LF-003", report.Warning, "eco/svc", "not in place",
		report.Remediation{Text: "link it", Command: "runthrough doctor --fix", Fixable: true})

	var buf bytes.Buffer
	if err := r.WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Command  string `json:"command"`
		Status   string `json:"status"`
		Findings []struct {
			Code        string `json:"code"`
			Severity    string `json:"severity"`
			Remediation *struct {
				Command string `json:"command"`
				Fixable bool   `json:"fixable"`
			} `json:"remediation"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("the output must be valid json: %v", err)
	}
	if len(got.Findings) != 1 {
		t.Fatalf("expected one finding, got %d", len(got.Findings))
	}
	f := got.Findings[0]
	if f.Code != "LF-003" || f.Severity != "warning" {
		t.Errorf("code %q severity %q", f.Code, f.Severity)
	}
	if f.Remediation == nil || !f.Remediation.Fixable || f.Remediation.Command == "" {
		t.Errorf("the remediation must survive serialization: %+v", f.Remediation)
	}
}

// A command that renders its own data must not get a stray "no findings"
// appended: in a log dump it reads as part of the service output.
func TestHumanViewDoesNotAppendToRenderedData(t *testing.T) {
	r := report.New("logs")
	r.Data = renderer{}
	var buf bytes.Buffer
	if err := r.WriteHuman(&buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "no findings") {
		t.Errorf("unexpected trailer: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "rendered") {
		t.Errorf("the command's own view must be written: %q", buf.String())
	}
}

type renderer struct{}

func (renderer) WriteHuman(w io.Writer) error {
	_, err := w.Write([]byte("rendered\n"))
	return err
}
