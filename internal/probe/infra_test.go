// SPDX-License-Identifier: GPL-3.0-or-later

package probe_test

import (
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/N3Y70R/runthrough/internal/probe"
)

// serve starts a listener that answers with reply after reading, or stays
// mute when reply is empty. It returns the port it bound.
func serve(t *testing.T, reply []byte, greet bool) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("no loopback listener available")
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(2 * time.Second))
				if greet {
					_, _ = c.Write(reply)
					return
				}
				buf := make([]byte, 64)
				if _, err := c.Read(buf); err != nil {
					return
				}
				if len(reply) > 0 {
					_, _ = c.Write(reply)
				}
			}(conn)
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

// The failure this whole check exists for: a socket that accepts connections
// and says nothing was being reported as a working Redis across two test
// rounds.
func TestAMuteListenerIsNotAService(t *testing.T) {
	port := serve(t, nil, false)

	got := probe.Check("redis", "127.0.0.1", port)
	if !got.Open {
		t.Fatal("the socket is open and that much should be reported")
	}
	if got.Spoke {
		t.Error("a listener that never answered PING must not count as redis")
	}
	if got.Detail == "" {
		t.Error("the report must say what happened, not just that it failed")
	}
}

func TestRedisAnswersItsOwnProtocol(t *testing.T) {
	for _, reply := range []string{"+PONG\r\n", "-NOAUTH Authentication required.\r\n"} {
		port := serve(t, []byte(reply), false)
		got := probe.Check("redis", "127.0.0.1", port)
		if !got.Spoke {
			t.Errorf("reply %q should prove it is redis, got %+v", reply, got)
		}
	}
}

func TestSomethingElseOnTheRedisPortIsCaught(t *testing.T) {
	port := serve(t, []byte("HTTP/1.1 200 OK\r\n\r\n"), false)
	got := probe.Check("redis", "127.0.0.1", port)
	if got.Spoke {
		t.Error("a web server on 6379 is not redis")
	}
}

func TestPostgresHandshakeNeedsNoCredentials(t *testing.T) {
	for _, answer := range []byte{'S', 'N'} {
		port := serve(t, []byte{answer}, false)
		got := probe.Check("postgres", "127.0.0.1", port)
		if !got.Spoke {
			t.Errorf("answer %q to SSLRequest should prove it is postgres, got %+v", answer, got)
		}
	}
}

func TestMysqlGreetingIsRead(t *testing.T) {
	// Length-prefixed greeting whose fifth byte is the protocol version.
	port := serve(t, []byte{0x0a, 0x00, 0x00, 0x00, 10}, true)
	if got := probe.Check("mysql", "127.0.0.1", port); !got.Spoke {
		t.Errorf("the greeting should prove it is mysql, got %+v", got)
	}
}

// An unknown component must not be reported as verified: the socket is all
// that was checked, and the report says exactly that.
func TestUnknownComponentReportsOnlyTheSocket(t *testing.T) {
	port := serve(t, nil, false)
	got := probe.Check("some-queue", "127.0.0.1", port)
	if !got.Open || got.Spoke || got.Protocol != "" {
		t.Errorf("expected socket-only verification, got %+v", got)
	}
	if got.Detail == "" {
		t.Error("it must say that only the socket was checked")
	}
}

func TestNothingListeningIsNotOpen(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("no loopback listener available")
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	if got := probe.Check("redis", "127.0.0.1", port); got.Open {
		t.Errorf("port %s has nothing on it, got %+v", strconv.Itoa(port), got)
	}
}
