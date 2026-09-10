// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import "testing"

// Compose has emitted both an array and one object per line, and the labels
// arrive as a single comma-separated string. Reading provenance out of that
// is what lets status say which commit is actually running.
func TestParseStatusAcceptsBothShapes(t *testing.T) {
	array := []byte(`[{"Service":"api","State":"running","Health":"healthy","Labels":"org.runthrough.commit=abc1234,org.runthrough.branch=main"}]`)
	lines := []byte(`{"Service":"api","State":"running","Health":"healthy","Labels":"org.runthrough.commit=abc1234"}
{"Service":"web","State":"exited","Health":"","Labels":""}`)

	fromArray := parseStatus(array)
	if len(fromArray) != 1 || fromArray["api"].State != "running" {
		t.Fatalf("array form: got %+v", fromArray)
	}
	if got := fromArray["api"].Label(LabelBranch); got != "main" {
		t.Errorf("branch label: got %q, want main", got)
	}

	fromLines := parseStatus(lines)
	if len(fromLines) != 2 {
		t.Fatalf("line form: got %d services", len(fromLines))
	}
	if got := fromLines["api"].Label(LabelCommit); got != "abc1234" {
		t.Errorf("commit label: got %q", got)
	}
	if got := fromLines["web"].Label(LabelCommit); got != "" {
		t.Errorf("a container with no labels must report an empty value, got %q", got)
	}
	if fromLines["web"].State != "exited" {
		t.Errorf("state: got %q", fromLines["web"].State)
	}
}

func TestParseStatusSurvivesNoise(t *testing.T) {
	if got := parseStatus([]byte("")); len(got) != 0 {
		t.Errorf("empty output must give no services, got %+v", got)
	}
	if got := parseStatus([]byte("not json at all\n")); len(got) != 0 {
		t.Errorf("unparseable output must give no services, got %+v", got)
	}
}
