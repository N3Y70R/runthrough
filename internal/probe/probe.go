// SPDX-License-Identifier: GPL-3.0-or-later

// Package probe answers the only question that matters after starting a
// stack: is it actually usable yet?
//
// A container that exists is not a service that answers. In one test run the
// tool reported success 145 seconds before the application served anything,
// which is worse than reporting nothing: it teaches people that a green
// result means nothing.
package probe

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"
)

// Target is one thing to wait for.
type Target struct {
	Service string
	Kind    string // http, tcp, or none
	URL     string
	Address string
	// Skip explains why this service cannot be probed from the host, which
	// is not a failure — a service with no published port is simply out of
	// reach from here.
	Skip string
}

// Result is what waiting found.
type Result struct {
	Service string `json:"service"`
	Kind    string `json:"kind"`
	Target  string `json:"target,omitempty"`
	Ready   bool   `json:"ready"`
	Skipped bool   `json:"skipped,omitempty"`
	Seconds int    `json:"seconds,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// Wait polls every target until it answers or the deadline passes. Targets
// are polled concurrently, so the total wait is the slowest one, not the sum.
func Wait(ctx context.Context, targets []Target, timeout time.Duration) []Result {
	results := make([]Result, len(targets))
	done := make(chan struct{}, len(targets))

	for i, t := range targets {
		go func(i int, t Target) {
			defer func() { done <- struct{}{} }()
			results[i] = waitOne(ctx, t, timeout)
		}(i, t)
	}
	for range targets {
		<-done
	}
	return results
}

func waitOne(ctx context.Context, t Target, timeout time.Duration) Result {
	r := Result{Service: t.Service, Kind: t.Kind}
	if t.Skip != "" {
		r.Skipped = true
		r.Detail = t.Skip
		return r
	}
	r.Target = t.Address
	if t.Kind == "http" {
		r.Target = t.URL
	}

	start := time.Now()
	deadline := start.Add(timeout)
	var last error
	for {
		var err error
		switch t.Kind {
		case "http":
			err = getOK(ctx, t.URL)
		default:
			err = dial(t.Address)
		}
		if err == nil {
			r.Ready = true
			r.Seconds = int(time.Since(start).Seconds())
			return r
		}
		last = err
		if time.Now().After(deadline) {
			r.Seconds = int(time.Since(start).Seconds())
			r.Detail = last.Error()
			return r
		}
		select {
		case <-ctx.Done():
			r.Detail = ctx.Err().Error()
			return r
		case <-time.After(time.Second):
		}
	}
}

func getOK(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Any answer means the service is up. A 404 from a running application
	// is a service that works; only a refused connection is "not yet".
	if resp.StatusCode >= 500 {
		return fmt.Errorf("answered %d", resp.StatusCode)
	}
	return nil
}

func dial(address string) error {
	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		return err
	}
	return conn.Close()
}

// Reachable reports whether something is listening at host:port. It is how
// infrastructure declared as living on the developer's machine gets checked
// instead of assumed.
func Reachable(host string, port int) error {
	return dial(net.JoinHostPort(host, fmt.Sprint(port)))
}

// PortTaken reports whether a host port is already in use, and by which
// family. It tries IPv4 and IPv6 separately, because a process bound to
// 127.0.0.1 does not always collide with a dual-stack listen, and missing
// that is how a port check gives false comfort.
//
// Only "address already in use" counts as taken. A host without IPv6 fails
// to listen for an unrelated reason, and reporting that as a busy port would
// make the check worse than useless: it would cry wolf on every port.
func PortTaken(port int) (bool, string) {
	for _, network := range []string{"tcp4", "tcp6"} {
		ln, err := net.Listen(network, fmt.Sprintf(":%d", port))
		if err == nil {
			ln.Close()
			continue
		}
		if inUse(err) {
			return true, network
		}
	}
	return false, ""
}

func inUse(err error) bool {
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	// Windows and some libcs report it with their own code but the same
	// wording, and a string check is the portable fallback.
	return strings.Contains(strings.ToLower(err.Error()), "address already in use") ||
		strings.Contains(strings.ToLower(err.Error()), "only one usage of each socket address")
}
