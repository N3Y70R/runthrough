// SPDX-License-Identifier: GPL-3.0-or-later

package probe_test

import (
	"net"
	"testing"

	"github.com/N3Y70R/runthrough/internal/probe"
)

// A host without IPv6 must not make every port look busy: a check that cries
// wolf on everything is worse than no check at all.
func TestPortTakenOnlyCountsAddressInUse(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("no loopback listener available")
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	if taken, family := probe.PortTaken(port); taken {
		t.Errorf("a free port reported as taken (%s)", family)
	}

	busy, err := net.Listen("tcp", net.JoinHostPort("0.0.0.0", itoa(port)))
	if err != nil {
		t.Skipf("cannot bind the port to test the positive case: %v", err)
	}
	defer busy.Close()

	if taken, _ := probe.PortTaken(port); !taken {
		t.Error("a port with a listener on it reported as free")
	}
}

func itoa(p int) string {
	return net.JoinHostPort("", "")[:0] + fmtInt(p)
}

func fmtInt(p int) string {
	if p == 0 {
		return "0"
	}
	var b []byte
	for p > 0 {
		b = append([]byte{byte('0' + p%10)}, b...)
		p /= 10
	}
	return string(b)
}
