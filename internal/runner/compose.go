// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import "context"

// Floor is the declared minimum for the Compose driver (D-12). include: is
// what the multi-ecosystem catalog relies on, and the engine of that era is
// where BuildKit became the default builder.
const (
	ComposeMinMajor = 2
	ComposeMinMinor = 20
	EngineMinMajor  = 24
	EngineMinMinor  = 0
)

// Compose drives Docker with the Compose plugin.
type Compose struct {
	// Binary is the runtime command, so the same driver can front a
	// Docker-compatible CLI without pretending it is Docker.
	Binary string
}

// NewCompose returns the default Docker-backed driver.
func NewCompose() *Compose { return &Compose{Binary: "docker"} }

// Name identifies the driver.
func (c *Compose) Name() string { return "compose" }

// Probe asks the runtime what it is and what it can do. It never guesses:
// when a version cannot be read, the capability is reported as absent with a
// reason, rather than assumed to be present.
func (c *Compose) Probe(ctx context.Context) Info {
	bin := c.Binary
	if bin == "" {
		bin = "docker"
	}
	info := Info{Driver: c.Name(), Capabilities: map[string]bool{}}

	client, err := Output(ctx, bin, "version", "--format", "{{.Client.Version}}")
	if err != nil {
		info.Reason = bin + " is not available on this machine"
		return info
	}
	info.Available = true
	info.Client = client

	if engine, err := Output(ctx, bin, "version", "--format", "{{.Server.Version}}"); err == nil {
		info.Engine = engine
	} else {
		info.Reason = "the engine is not answering: is the daemon running?"
	}

	compose, err := Output(ctx, bin, "compose", "version", "--short")
	if err != nil {
		info.Reason = "the compose plugin is not installed"
		return info
	}
	info.Compose = compose

	composeOK := AtLeast(compose, ComposeMinMajor, ComposeMinMinor)
	engineOK := info.Engine == "" || AtLeast(info.Engine, EngineMinMajor, EngineMinMinor)

	info.Capabilities[string(CapInclude)] = composeOK
	info.Capabilities[string(CapDependsCondition)] = composeOK
	info.Capabilities[string(CapSSHBuildMount)] = composeOK && engineOK
	info.Capabilities[string(CapBindMount)] = true
	info.Capabilities[string(CapHostAlias)] = true
	return info
}
