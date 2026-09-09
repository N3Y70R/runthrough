// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"fmt"
	"io"
	"sort"

	"github.com/N3Y70R/runthrough/internal/catalog"
	"github.com/N3Y70R/runthrough/internal/config"
	"github.com/N3Y70R/runthrough/internal/report"
)

func runConfig(e *env, args []string) (*report.Result, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("expected a subcommand: show or validate")
	}
	sub := args[0]
	fs := e.flags("config " + sub)
	if _, err := parse(fs, args[1:]); err != nil {
		return nil, err
	}

	switch sub {
	case "show":
		return configShow(e)
	case "validate":
		return configValidate(e)
	default:
		return nil, fmt.Errorf("unknown subcommand %q: expected show or validate", sub)
	}
}

func configShow(e *env) (*report.Result, error) {
	lookup, user, err := e.lookup()
	if err != nil {
		return nil, err
	}
	res, err := config.ResolveCatalog(lookup)
	if err != nil {
		return nil, err
	}
	cat, err := catalog.Load(res.Path)
	if err != nil {
		return nil, err
	}

	data := &showData{
		Catalog:        res,
		Manifest:       cat.Path,
		UserConfig:     user.Path,
		UserConfigSet:  user.Exists,
		SnapshotsRoot:  user.SnapshotsRoot(),
		LocalFileStore: cat.Defaults.LocalFiles.Store,
		LocalFileMode:  cat.Defaults.LocalFiles.Mode,
	}
	for _, name := range sortedNames(cat.Ecosystems) {
		eco := cat.Ecosystems[name]
		data.Ecosystems = append(data.Ecosystems, ecosystemSummary{
			Name:            name,
			Workspace:       eco.Workspace,
			DefaultWorktree: eco.DefaultWorktree,
			Services:        len(eco.Services),
			GatewayPort:     gatewayPort(eco),
		})
	}
	for name := range cat.Infra.Profiles {
		data.InfraProfiles = append(data.InfraProfiles, name)
	}
	sort.Strings(data.InfraProfiles)

	r := report.New("config show")
	r.Data = data
	return r, nil
}

func configValidate(e *env) (*report.Result, error) {
	lookup, _, err := e.lookup()
	if err != nil {
		return nil, err
	}
	res, err := config.ResolveCatalog(lookup)
	if err != nil {
		return nil, err
	}
	cat, err := catalog.Load(res.Path)
	if err != nil {
		return nil, err
	}

	r := report.New("config validate")
	for _, f := range cat.Validate() {
		r.Add(f)
	}
	r.Data = &validateData{Manifest: cat.Path, Source: string(res.Source), Ecosystems: len(cat.Ecosystems), Services: countServices(cat)}
	return r, nil
}

type showData struct {
	Catalog        config.Resolution  `json:"catalog"`
	Manifest       string             `json:"manifest"`
	UserConfig     string             `json:"user_config"`
	UserConfigSet  bool               `json:"user_config_exists"`
	SnapshotsRoot  string             `json:"snapshots_root"`
	LocalFileStore string             `json:"local_file_store"`
	LocalFileMode  string             `json:"local_file_mode"`
	Ecosystems     []ecosystemSummary `json:"ecosystems"`
	InfraProfiles  []string           `json:"infra_profiles,omitempty"`
}

type ecosystemSummary struct {
	Name            string `json:"name"`
	Workspace       string `json:"workspace"`
	DefaultWorktree string `json:"default_worktree,omitempty"`
	Services        int    `json:"services"`
	GatewayPort     int    `json:"gateway_port,omitempty"`
}

func (d *showData) WriteHuman(w io.Writer) error {
	fmt.Fprintf(w, "catalog        %s\n", d.Manifest)
	fmt.Fprintf(w, "found via      %s", d.Catalog.Source)
	if d.Catalog.ScannedFrom != "" {
		fmt.Fprintf(w, " (scanning up from %s)", d.Catalog.ScannedFrom)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "user config    %s", d.UserConfig)
	if !d.UserConfigSet {
		fmt.Fprint(w, "  (not present, defaults apply)")
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "snapshots      %s\n", d.SnapshotsRoot)
	fmt.Fprintf(w, "local files    %s (%s)\n", d.LocalFileStore, d.LocalFileMode)
	if len(d.InfraProfiles) > 0 {
		fmt.Fprintf(w, "infra profiles %v\n", d.InfraProfiles)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%-14s %-9s %-18s %s\n", "ECOSYSTEM", "SERVICES", "WORKTREE", "WORKSPACE")
	for _, eco := range d.Ecosystems {
		fmt.Fprintf(w, "%-14s %-9d %-18s %s\n", eco.Name, eco.Services, dash(eco.DefaultWorktree), eco.Workspace)
	}
	return nil
}

type validateData struct {
	Manifest   string `json:"manifest"`
	Source     string `json:"source"`
	Ecosystems int    `json:"ecosystems"`
	Services   int    `json:"services"`
}

func (d *validateData) WriteHuman(w io.Writer) error {
	_, err := fmt.Fprintf(w, "validated %s\n%d ecosystem(s), %d service(s)\n", d.Manifest, d.Ecosystems, d.Services)
	return err
}

func countServices(c *catalog.Catalog) int {
	n := 0
	for _, eco := range c.Ecosystems {
		n += len(eco.Services)
	}
	return n
}

func gatewayPort(e *catalog.Ecosystem) int {
	if e.Gateway == nil {
		return 0
	}
	return e.Gateway.Port
}

func sortedNames(m map[string]*catalog.Ecosystem) []string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
