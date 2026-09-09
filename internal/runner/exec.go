// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
)

// Output runs a command and returns its trimmed stdout. Errors are the
// caller's to interpret: a missing binary and a failing flag mean different
// things.
func Output(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// AtLeast compares dotted versions leniently: it reads as many numeric
// components as both sides share and ignores any suffix. Runtimes report
// versions in creative shapes, so a strict parser would reject more than it
// catches.
func AtLeast(got string, major, minor int) bool {
	gotMajor, gotMinor, ok := parse(got)
	if !ok {
		return false
	}
	if gotMajor != major {
		return gotMajor > major
	}
	return gotMinor >= minor
}

func parse(v string) (int, int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" {
		return 0, 0, false
	}
	parts := strings.Split(v, ".")
	major, err := strconv.Atoi(digits(parts[0]))
	if err != nil {
		return 0, 0, false
	}
	minor := 0
	if len(parts) > 1 {
		minor, _ = strconv.Atoi(digits(parts[1]))
	}
	return major, minor, true
}

func digits(s string) string {
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	return s[:end]
}
