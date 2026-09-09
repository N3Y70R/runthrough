// SPDX-License-Identifier: GPL-3.0-or-later

package probe

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"time"
)

// Reach is the outcome of checking an infrastructure component.
type Reach struct {
	// Open means something accepted the connection.
	Open bool
	// Spoke means the thing that accepted it answered its own protocol.
	// A socket that opens and stays mute is not a database.
	Spoke bool
	// Protocol names what was spoken, or is empty when only the socket was
	// checked because this component has no known handshake.
	Protocol string
	Detail   string
}

// Check verifies a component the way the component itself would be used.
//
// A TCP connect proves that something is listening, not that it is the thing
// you need: a dead SSH tunnel, a stale port-forward or a database still
// starting up all accept connections and say nothing. The same reasoning that
// made a tcp readiness probe untrustworthy applies here, and it took two test
// rounds to apply it in both places.
func Check(component, host string, port int) Reach {
	address := net.JoinHostPort(host, fmt.Sprint(port))
	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		return Reach{Detail: err.Error()}
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))

	r := Reach{Open: true}
	switch normalize(component) {
	case "redis":
		r.Protocol = "redis"
		r.Spoke, r.Detail = speaksRedis(conn)
	case "postgres", "postgresql":
		r.Protocol = "postgres"
		r.Spoke, r.Detail = speaksPostgres(conn)
	case "mysql", "mariadb":
		r.Protocol = "mysql"
		r.Spoke, r.Detail = speaksMySQL(conn)
	default:
		// No known handshake: the socket is all that can be checked, and
		// the report says so rather than implying more.
		r.Detail = "socket open; no protocol check known for this component"
	}
	return r
}

// speaksRedis sends PING and expects PONG. It needs no credentials: a server
// with authentication answers with an error, which still proves it is Redis.
func speaksRedis(conn net.Conn) (bool, string) {
	if _, err := conn.Write([]byte("PING\r\n")); err != nil {
		return false, err.Error()
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return false, "connected, but nothing answered PING"
	}
	line = strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(line, "+PONG"):
		return true, ""
	case strings.HasPrefix(line, "-NOAUTH"), strings.HasPrefix(line, "-ERR"):
		// It is Redis, and it is asking for credentials. Being reachable is
		// what was asked; its access rules are not this tool's business.
		return true, "answered, authentication required"
	default:
		return false, "answered something that is not the redis protocol"
	}
}

// speaksPostgres sends an SSLRequest, the one message a server answers before
// authentication: 'S' or 'N', a single byte. No credentials, no side effects.
func speaksPostgres(conn net.Conn) (bool, string) {
	msg := make([]byte, 8)
	binary.BigEndian.PutUint32(msg[0:4], 8)
	binary.BigEndian.PutUint32(msg[4:8], 80877103) // SSLRequest
	if _, err := conn.Write(msg); err != nil {
		return false, err.Error()
	}
	answer := make([]byte, 1)
	if _, err := conn.Read(answer); err != nil {
		return false, "connected, but nothing answered the postgres handshake"
	}
	if answer[0] == 'S' || answer[0] == 'N' {
		return true, ""
	}
	return false, "answered something that is not the postgres protocol"
}

// speaksMySQL reads the greeting the server sends unprompted on connect.
func speaksMySQL(conn net.Conn) (bool, string) {
	head := make([]byte, 5)
	if _, err := conn.Read(head); err != nil {
		return false, "connected, but no mysql greeting arrived"
	}
	// Byte 4 is the protocol version; 10 is the modern handshake.
	if head[4] == 10 || head[4] == 9 {
		return true, ""
	}
	return false, "answered something that is not the mysql protocol"
}

func normalize(component string) string {
	return strings.ToLower(strings.TrimSpace(component))
}
