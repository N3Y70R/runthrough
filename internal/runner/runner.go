// SPDX-License-Identifier: GPL-3.0-or-later

// Package runner is the port between the domain and whatever actually runs
// the containers.
//
// The domain speaks of services, dependencies and exposure; a driver
// translates that to its runtime and, just as important, declares what it can
// and cannot do (D-8, R-01, R-03). A stack that starts halfway because the
// driver silently lacked something is the worst failure mode there is.
package runner

import "context"

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

// Driver is a container runtime this tool can drive. Phase 0 only probes;
// bringing services up arrives with phase 1.
type Driver interface {
	Name() string
	Probe(ctx context.Context) Info
}
