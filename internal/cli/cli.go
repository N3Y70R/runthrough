// SPDX-License-Identifier: GPL-3.0-or-later

// Package cli is the human-facing frontend. It holds no logic of its own: it
// parses arguments, calls the domain and renders the result (principle 1).
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/N3Y70R/runthrough/internal/config"
	"github.com/N3Y70R/runthrough/internal/report"
	"github.com/N3Y70R/runthrough/internal/version"
)

type env struct {
	stdout io.Writer
	stderr io.Writer
	json   bool
	// catalogFlag is the --catalog value, first link of the resolution chain.
	catalogFlag string
}

type command struct {
	name    string
	summary string
	usage   string
	run     func(e *env, args []string) (*report.Result, error)
}

func commands() []command {
	return []command{
		{
			name:    "config",
			summary: "show or validate the catalog in use",
			usage:   "runthrough config show|validate [--catalog PATH] [--json]",
			run:     runConfig,
		},
		{
			name:    "version",
			summary: "print the build identity",
			usage:   "runthrough version",
			run:     runVersion,
		},
	}
}

// Main runs the CLI and returns the process exit code: 0 when nothing is
// broken, 1 when a command reports errors, 2 for a usage mistake.
func Main(args []string, stdout, stderr io.Writer) int {
	e := &env{stdout: stdout, stderr: stderr}

	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "help":
		usage(stdout)
		return 0
	case "-v", "--version":
		args = []string{"version"}
	}

	name := args[0]
	for _, c := range commands() {
		if c.name != name {
			continue
		}
		result, err := c.run(e, args[1:])
		if err != nil {
			fmt.Fprintf(stderr, "runthrough %s: %v\n", name, err)
			return 2
		}
		if err := e.render(result); err != nil {
			fmt.Fprintf(stderr, "runthrough: %v\n", err)
			return 2
		}
		return result.ExitCode()
	}

	fmt.Fprintf(stderr, "runthrough: unknown command %q\n\n", name)
	usage(stderr)
	return 2
}

func (e *env) render(r *report.Result) error {
	if e.json {
		return r.WriteJSON(e.stdout)
	}
	return r.WriteHuman(e.stdout)
}

// flags builds a flag set carrying the options every command shares.
func (e *env) flags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	fs.BoolVar(&e.json, "json", false, "emit the structured result instead of the human view")
	fs.StringVar(&e.catalogFlag, "catalog", "", "path to the catalog manifest or directory")
	return fs
}

// lookup assembles the resolution chain inputs (D-9).
func (e *env) lookup() (config.Lookup, *config.User, error) {
	user, err := config.LoadUser()
	if err != nil {
		return config.Lookup{}, nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}
	return config.Lookup{
		Flag: e.catalogFlag,
		Env:  os.Getenv(config.EnvCatalog),
		Cwd:  cwd,
		User: user,
	}, user, nil
}

func runVersion(e *env, args []string) (*report.Result, error) {
	fs := e.flags("version")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	r := report.New("version")
	r.Data = &versionData{
		Version: version.Version,
		Commit:  version.Commit,
		Date:    version.Date,
	}
	return r, nil
}

type versionData struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

func (v *versionData) WriteHuman(w io.Writer) error {
	_, err := fmt.Fprintln(w, version.String())
	return err
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "runthrough — run your whole stack the way it will actually run")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "usage: runthrough <command> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "commands:")
	cs := commands()
	sort.Slice(cs, func(i, j int) bool { return cs[i].name < cs[j].name })
	for _, c := range cs {
		fmt.Fprintf(w, "  %-10s %s\n", c.name, c.summary)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "global flags:")
	fmt.Fprintln(w, "  --catalog PATH   catalog manifest or directory to work with")
	fmt.Fprintln(w, "  --json           emit the structured result instead of the human view")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "the catalog is resolved in this order: --catalog, %s,\n", config.EnvCatalog)
	fmt.Fprintln(w, "a runthrough.yaml found upwards from the current directory, then the")
	fmt.Fprintln(w, "default registered in the user config.")
}

func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
}
