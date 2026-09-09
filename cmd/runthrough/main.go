// SPDX-License-Identifier: GPL-3.0-or-later

// Command runthrough brings up a service ecosystem locally, in containers,
// built from git worktrees.
package main

import (
	"os"

	"github.com/N3Y70R/runthrough/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr))
}
