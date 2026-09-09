// SPDX-License-Identifier: GPL-3.0-or-later

// Package report holds the structured result every operation returns.
//
// The JSON form is the primary output (T-04): an agent reasons over data, not
// over prose. The human rendering is a view of the same structure, never a
// separate code path.
package report

import (
	"encoding/json"
	"fmt"
	"io"
)

// Severity classifies a finding.
type Severity string

const (
	Info    Severity = "info"
	Warning Severity = "warning"
	Error   Severity = "error"
)

// Remediation tells the reader — human or agent — what to do about a finding.
// Fixable marks the ones `--fix` can resolve on its own (S-06).
type Remediation struct {
	Text    string `json:"text"`
	Command string `json:"command,omitempty"`
	Fixable bool   `json:"fixable"`
}

// Finding is one diagnosis. Code is stable across releases so that agents and
// scripts can branch on it; Message is for people and may be reworded (T-10).
type Finding struct {
	Code        string       `json:"code"`
	Severity    Severity     `json:"severity"`
	Scope       string       `json:"scope,omitempty"`
	Message     string       `json:"message"`
	Remediation *Remediation `json:"remediation,omitempty"`
}

// Result is what a command returns.
type Result struct {
	Command  string    `json:"command"`
	Status   string    `json:"status"`
	Findings []Finding `json:"findings,omitempty"`
	Data     any       `json:"data,omitempty"`
}

// HumanWriter lets a command's Data render itself for people. Data that does
// not implement it is simply omitted from the human view.
type HumanWriter interface {
	WriteHuman(w io.Writer) error
}

// New starts an empty, successful result.
func New(command string) *Result {
	return &Result{Command: command, Status: "ok"}
}

// Add records a finding and updates the overall status.
func (r *Result) Add(f Finding) {
	r.Findings = append(r.Findings, f)
	switch f.Severity {
	case Error:
		r.Status = "error"
	case Warning:
		if r.Status == "ok" {
			r.Status = "warning"
		}
	}
}

// Addf records a finding with a formatted message.
func (r *Result) Addf(code string, sev Severity, scope, format string, args ...any) {
	r.Add(Finding{Code: code, Severity: sev, Scope: scope, Message: fmt.Sprintf(format, args...)})
}

// Fix records a finding that carries a remediation.
func (r *Result) Fix(code string, sev Severity, scope, msg string, rem Remediation) {
	r.Add(Finding{Code: code, Severity: sev, Scope: scope, Message: msg, Remediation: &rem})
}

// HasErrors reports whether any finding is an error.
func (r *Result) HasErrors() bool { return r.Status == "error" }

// ExitCode is 1 when something is broken, 0 otherwise. Warnings do not fail:
// a warning that blocks a command trains people to ignore warnings.
func (r *Result) ExitCode() int {
	if r.HasErrors() {
		return 1
	}
	return 0
}

// WriteJSON emits the machine-readable form.
func (r *Result) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// WriteHuman emits the readable form: the command's own summary first, then
// the findings, then a one-line tally.
func (r *Result) WriteHuman(w io.Writer) error {
	rendered := false
	if hw, ok := r.Data.(HumanWriter); ok {
		if err := hw.WriteHuman(w); err != nil {
			return err
		}
		rendered = true
	}
	if len(r.Findings) == 0 {
		if rendered {
			return nil
		}
		_, err := fmt.Fprintln(w, "no findings")
		return err
	}
	fmt.Fprintln(w)
	for _, f := range r.Findings {
		scope := f.Scope
		if scope != "" {
			scope = "  " + scope
		}
		fmt.Fprintf(w, "  %-7s %s%s  %s\n", label(f.Severity), f.Code, scope, f.Message)
		if f.Remediation != nil {
			fmt.Fprintf(w, "          -> %s\n", f.Remediation.Text)
			if f.Remediation.Command != "" {
				fmt.Fprintf(w, "             %s\n", f.Remediation.Command)
			}
		}
	}
	var errs, warns int
	for _, f := range r.Findings {
		switch f.Severity {
		case Error:
			errs++
		case Warning:
			warns++
		}
	}
	_, err := fmt.Fprintf(w, "\n%d finding(s): %d error(s), %d warning(s)\n", len(r.Findings), errs, warns)
	return err
}

func label(s Severity) string {
	switch s {
	case Error:
		return "ERROR"
	case Warning:
		return "WARN"
	default:
		return "INFO"
	}
}
