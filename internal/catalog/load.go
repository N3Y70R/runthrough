// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ManifestName is the file a catalog directory is recognised by.
const ManifestName = "runthrough.yaml"

// Load reads a catalog from a manifest file or from a directory containing
// one, resolves its includes and applies defaults.
func Load(path string) (*Catalog, error) {
	manifest, err := manifestPath(path)
	if err != nil {
		return nil, err
	}

	c, err := decodeFile(manifest)
	if err != nil {
		return nil, err
	}
	c.Path = manifest
	c.Dir = filepath.Dir(manifest)

	for _, inc := range c.Include {
		p := c.Resolve(inc)
		frag, err := decodeFile(p)
		if err != nil {
			return nil, fmt.Errorf("include %s: %w", inc, err)
		}
		if err := c.merge(frag, inc); err != nil {
			return nil, err
		}
	}

	c.applyDefaults()
	return c, nil
}

// Resolve turns a path from the manifest into an absolute one. Relative paths
// resolve against the manifest, never against the working directory (D-9).
func (c *Catalog) Resolve(p string) string {
	if p == "" {
		return ""
	}
	if expanded, err := expandHome(p); err == nil {
		p = expanded
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(c.Dir, p)
}

func manifestPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("no catalog path given")
	}
	expanded, err := expandHome(path)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("catalog not found at %s", abs)
	}
	if info.IsDir() {
		abs = filepath.Join(abs, ManifestName)
		if _, err := os.Stat(abs); err != nil {
			return "", fmt.Errorf("no %s in %s", ManifestName, filepath.Dir(abs))
		}
	}
	return abs, nil
}

func decodeFile(path string) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Catalog
	dec := yaml.NewDecoder(newReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return &c, nil
}

func (c *Catalog) merge(frag *Catalog, origin string) error {
	if c.Ecosystems == nil {
		c.Ecosystems = map[string]*Ecosystem{}
	}
	for name, eco := range frag.Ecosystems {
		if _, exists := c.Ecosystems[name]; exists {
			return fmt.Errorf("include %s redefines ecosystem %q", origin, name)
		}
		c.Ecosystems[name] = eco
	}
	if frag.Infra.Profiles != nil {
		if c.Infra.Profiles == nil {
			c.Infra.Profiles = map[string]map[string]string{}
		}
		for name, profile := range frag.Infra.Profiles {
			if _, exists := c.Infra.Profiles[name]; exists {
				return fmt.Errorf("include %s redefines infra profile %q", origin, name)
			}
			c.Infra.Profiles[name] = profile
		}
	}
	return nil
}

// applyDefaults fills in what the manifest left implicit: names, the local
// file store and mode. It never invents a worktree or a port.
func (c *Catalog) applyDefaults() {
	if c.Defaults.LocalFiles.Store == "" {
		c.Defaults.LocalFiles.Store = "{repo}/artifacts"
	}
	if c.Defaults.LocalFiles.Mode == "" {
		c.Defaults.LocalFiles.Mode = "link"
	}
	for ecoName, eco := range c.Ecosystems {
		eco.Name = ecoName
		for svcName, svc := range eco.Services {
			svc.Name = svcName
			if svc.Repo == "" && svc.Image == "" {
				svc.Repo = svcName
			}
			for i := range svc.LocalFiles {
				if svc.LocalFiles[i].Mode == "" {
					svc.LocalFiles[i].Mode = c.Defaults.LocalFiles.Mode
				}
			}
		}
	}
}

func expandHome(p string) (string, error) {
	if p == "" || p[0] != '~' {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p, err
	}
	if p == "~" {
		return home, nil
	}
	if len(p) > 1 && (p[1] == '/' || p[1] == filepath.Separator) {
		return filepath.Join(home, p[2:]), nil
	}
	return p, nil
}
