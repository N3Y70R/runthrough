// SPDX-License-Identifier: GPL-3.0-or-later

// Package version carries the build identity, injected with -ldflags.
package version

import "fmt"

var (
	// Version is the release version, or "dev" for a local build.
	Version = "dev"
	// Commit is the short commit the binary was built from.
	Commit = "none"
	// Date is the build timestamp in RFC 3339.
	Date = "unknown"
)

// String renders the full build identity.
func String() string {
	return fmt.Sprintf("runthrough %s (commit %s, built %s)", Version, Commit, Date)
}
