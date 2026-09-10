// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
)

// ServiceState is what the runtime says about one container.
type ServiceState struct {
	Name   string `json:"Service"`
	State  string `json:"State"`
	Health string `json:"Health"`
	// Labels carries what was stamped on the image when it was built. It is
	// how a running container can be traced back to the commit it came from,
	// which is the difference between a stack you can measure and one you
	// can only look at.
	Labels string `json:"Labels"`
}

// Label reads one stamped value.
func (s ServiceState) Label(name string) string {
	for _, pair := range strings.Split(s.Labels, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if ok && key == name {
			return value
		}
	}
	return ""
}

// Provenance labels, written by the artifact from the resolved plan.
const (
	LabelRepo   = "org.runthrough.repo"
	LabelBranch = "org.runthrough.branch"
	LabelCommit = "org.runthrough.commit"
)

// Status asks the runtime what is running. The runtime's own healthcheck is
// the only trustworthy readiness signal: it runs inside the container, so it
// cannot be fooled by a port the runtime published before the process behind
// it was listening.
func (c *Compose) Status(ctx context.Context, inv Invocation) (map[string]ServiceState, error) {
	bin := c.Binary
	if bin == "" {
		bin = "docker"
	}
	args := append([]string{"compose"}, append(c.base(inv), "ps", "--format", "json", "--all")...)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = environ(inv.Env)
	cmd.Dir = inv.Dir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parseStatus(out), nil
}

// parseStatus accepts both shapes Compose has used: a JSON array, and one
// JSON object per line.
func parseStatus(out []byte) map[string]ServiceState {
	states := map[string]ServiceState{}

	var list []ServiceState
	if err := json.Unmarshal(out, &list); err == nil {
		for _, s := range list {
			states[s.Name] = s
		}
		return states
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var s ServiceState
		if err := json.Unmarshal([]byte(line), &s); err == nil && s.Name != "" {
			states[s.Name] = s
		}
	}
	return states
}
