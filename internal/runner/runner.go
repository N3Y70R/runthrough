// SPDX-License-Identifier: GPL-3.0-or-later

// Package runner is the port between the domain and whatever actually runs
// the containers.
//
// The domain speaks of services, dependencies and exposure; a driver
// translates that to its runtime and, just as important, declares what it can
// and cannot do (D-8, R-01, R-03). A stack that starts halfway because the
// driver silently lacked something is the worst failure mode there is.
package runner

import (
	"context"
	"io"
)

// Capability is something a catalog may rely on and a driver may lack.
type Capability string

const (
	// CapInclude composes a catalog from several files (D-1).
	CapInclude Capability = "include"
	// CapDependsCondition waits for health or completion before starting a
	// dependent service (H-02).
	CapDependsCondition Capability = "depends-condition"
	// CapSSHBuildMount lends the SSH agent to a build without baking
	// credentials into the image (B-01).
	CapSSHBuildMount Capability = "ssh-build-mount"
	// CapBindMount shares the working tree with the container, which is what
	// makes hot reload possible (B-05).
	CapBindMount Capability = "bind-mount"
	// CapHostAlias lets a container reach a service running on the host
	// machine (N-06).
	CapHostAlias Capability = "host-alias"
)

// Required lists the capabilities the tool assumes today.
func Required() []Capability {
	return []Capability{CapInclude, CapDependsCondition, CapSSHBuildMount, CapBindMount, CapHostAlias}
}

// Info is the outcome of probing a driver.
type Info struct {
	Driver       string          `json:"driver"`
	Available    bool            `json:"available"`
	Experimental bool            `json:"experimental,omitempty"`
	Client       string          `json:"client_version,omitempty"`
	Engine       string          `json:"engine_version,omitempty"`
	Compose      string          `json:"compose_version,omitempty"`
	Capabilities map[string]bool `json:"capabilities,omitempty"`
	Reason       string          `json:"reason,omitempty"`
}

// Supports reports whether the probed runtime offers a capability.
func (i Info) Supports(c Capability) bool { return i.Capabilities[string(c)] }

// Invocation is everything a driver needs to act on one ecosystem. It is
// deliberately runtime-neutral in spirit: a driver that is not Compose reads
// the same fields and does whatever its runtime requires.
type Invocation struct {
	// Project namespaces the running stack, so two ecosystems can run side
	// by side without colliding (N-03).
	Project string
	// File is the driver artifact: readable and runnable by hand, which is
	// the escape hatch (principle 3).
	File string
	// Dir is what relative paths inside that artifact resolve against.
	Dir string
	// Env carries the resolved plan: worktree paths, ports, infrastructure
	// endpoints and provenance labels.
	Env map[string]string
	// EnvFile holds the same values on disk, so the command the tool prints
	// is the command a person can paste (principle 3).
	EnvFile string
}

// UpOptions tunes bringing services up.
type UpOptions struct {
	Services []string
	Build    bool
}

// DownOptions tunes taking them down. Data is kept unless explicitly dropped:
// deleting someone's local database must be something they asked for.
type DownOptions struct {
	Services []string
	Volumes  bool
}

// Driver is a container runtime this tool can drive.
type Driver interface {
	Name() string
	Probe(ctx context.Context) Info
	Up(ctx context.Context, inv Invocation, opts UpOptions, stdout, stderr io.Writer) error
	Down(ctx context.Context, inv Invocation, opts DownOptions, stdout, stderr io.Writer) error
	Stop(ctx context.Context, inv Invocation, services []string, stdout, stderr io.Writer) error
	Status(ctx context.Context, inv Invocation) (map[string]ServiceState, error)
	Build(ctx context.Context, inv Invocation, services []string, stdout, stderr io.Writer) error
	Logs(ctx context.Context, inv Invocation, services []string, follow bool, stdout, stderr io.Writer) error
}
