// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
)

// Up brings services up in the background, optionally building first.
func (c *Compose) Up(ctx context.Context, inv Invocation, opts UpOptions, stdout, stderr io.Writer) error {
	args := []string{"up", "--detach", "--remove-orphans"}
	if opts.Build {
		args = append(args, "--build")
	}
	args = append(args, opts.Services...)
	return c.run(ctx, inv, args, stdout, stderr)
}

// Down stops the stack. Volumes survive unless the caller says otherwise.
func (c *Compose) Down(ctx context.Context, inv Invocation, opts DownOptions, stdout, stderr io.Writer) error {
	args := []string{"down", "--remove-orphans"}
	if opts.Volumes {
		args = append(args, "--volumes")
	}
	return c.run(ctx, inv, args, stdout, stderr)
}

// Build rebuilds images without starting anything.
func (c *Compose) Build(ctx context.Context, inv Invocation, services []string, stdout, stderr io.Writer) error {
	return c.run(ctx, inv, append([]string{"build"}, services...), stdout, stderr)
}

// Logs streams service output.
func (c *Compose) Logs(ctx context.Context, inv Invocation, services []string, follow bool, stdout, stderr io.Writer) error {
	args := []string{"logs", "--no-log-prefix=false"}
	if follow {
		args = append(args, "--follow")
	} else {
		args = append(args, "--tail", "200")
	}
	return c.run(ctx, inv, append(args, services...), stdout, stderr)
}

// Command renders the equivalent command line. It is what makes the escape
// hatch real: whatever the tool does, a person can reproduce by hand.
func (c *Compose) Command(inv Invocation, args []string) string {
	return fmt.Sprintf("docker compose -p %s -f %s --project-directory %s %s",
		inv.Project, inv.File, inv.Dir, join(args))
}

func (c *Compose) run(ctx context.Context, inv Invocation, args []string, stdout, stderr io.Writer) error {
	bin := c.Binary
	if bin == "" {
		bin = "docker"
	}
	full := append([]string{
		"compose",
		"--project-name", inv.Project,
		"--file", inv.File,
		"--project-directory", inv.Dir,
	}, args...)

	cmd := exec.CommandContext(ctx, bin, full...)
	cmd.Env = environ(inv.Env)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Dir = inv.Dir
	return cmd.Run()
}

// environ merges the resolved plan into the ambient environment. The plan
// wins: the whole point is that what the catalog resolved is what runs.
func environ(extra map[string]string) []string {
	env := os.Environ()
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		env = append(env, k+"="+extra[k])
	}
	return env
}

func join(args []string) string {
	out := ""
	for i, a := range args {
		if i > 0 {
			out += " "
		}
		out += a
	}
	return out
}
