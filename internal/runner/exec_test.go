// SPDX-License-Identifier: GPL-3.0-or-later

package runner_test

import (
	"testing"

	"github.com/N3Y70R/runthrough/internal/runner"
)

func TestAtLeastReadsCreativeVersionStrings(t *testing.T) {
	cases := []struct {
		got   string
		major int
		minor int
		want  bool
	}{
		{"2.20.0", 2, 20, true},
		{"v2.29.7", 2, 20, true},
		{"2.19.1", 2, 20, false},
		{"3.0.0", 2, 20, true},
		{"1.29.2", 2, 20, false},
		{"24.0.7", 24, 0, true},
		{"23.0.5", 24, 0, false},
		{"2.20.0-desktop.1", 2, 20, true},
		{"", 2, 20, false},
		{"unknown", 2, 20, false},
	}
	for _, c := range cases {
		if got := runner.AtLeast(c.got, c.major, c.minor); got != c.want {
			t.Errorf("AtLeast(%q, %d, %d) = %v, want %v", c.got, c.major, c.minor, got, c.want)
		}
	}
}
