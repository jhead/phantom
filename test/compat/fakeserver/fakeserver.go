// Package fakeserver is a scriptable RakNet upstream for the black-box tier.
//
// It exists because a real Minecraft server cannot be made to emit a truncated
// pong, a 30-field MOTD, or a pong from a protocol version that does not exist
// yet. This one can, byte for byte.
//
// It is a test double standing in for the REMOTE server. phantom itself is
// always a real subprocess - see test/compat/harness.go.
package fakeserver

import (
	"bytes"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/jhead/phantom/internal/corpus"
)

// Opts configures a fake upstream.
type Opts struct {
	// Pong is the exact datagram to answer offline pings with. If nil, one is
	// built from MOTD.
	Pong []byte

	// MOTD builds a well-formed pong when Pong is nil.
	MOTD string

	// EchoPayloads makes the server return any non-ping datagram verbatim,
	// which is how payload-integrity and MTU cases are measured.
	EchoPayloads bool

	// SilentToPings makes the server ignore pings entirely, simulating an
	// upstream that is reachable but not answering.
	SilentToPings bool
}

// Server is a fake RakNet upstream listening on loopback.
type Server struct {
	conn *net.UDPConn
	opts Opts

	mu       sync.Mutex
	received [][]byte
	pings    int
	stopped  bool

	done chan struct{}
	wg   sync.WaitGroup
}

// Start binds a fake upstream to an ephemeral loopback port.
func Start(opts Opts) (*Server, error) {
	if opts.Pong == nil {
		motd := opts.MOTD
		if motd == "" {
			motd = "MCPE;Fake Upstream;800;1.21.80;0;10;1234567890;Sub;Survival;1;19132;19133;0;"
		}
		opts.Pong = corpus.BuildPong(
			make([]byte, 8), // ping time; overwritten per-request with the caller's
			[]byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x01, 0x02},
			motd,
		)
	}

	addr, err := net.ResolveUDPAddr("udp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, err
	}

	s := &Server{conn: conn, opts: opts, done: make(chan struct{})}
	s.wg.Add(1)
	go s.serve()
	return s, nil
}

// Addr is the host:port phantom should be pointed at with -server.
func (s *Server) Addr() string { return s.conn.LocalAddr().String() }

func (s *Server) serve() {
	defer s.wg.Done()
	buf := make([]byte, 4096)

	for {
		select {
		case <-s.done:
			return
		default:
		}

		_ = s.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, from, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return
		}

		data := append([]byte(nil), buf[:n]...)

		s.mu.Lock()
		s.received = append(s.received, data)
		stopped := s.stopped
		s.mu.Unlock()

		if stopped || n == 0 {
			continue
		}

		if isOfflinePing(data) {
			s.mu.Lock()
			s.pings++
			s.mu.Unlock()

			if s.opts.SilentToPings {
				continue
			}
			_, _ = s.conn.WriteToUDP(s.pongFor(data), from)
			continue
		}

		if s.opts.EchoPayloads {
			_, _ = s.conn.WriteToUDP(data, from)
		}
	}
}

// pingMagicOffset is where the offline magic sits in an Unconnected Ping:
// after the packet ID (1) and the ping time (8). Note this is NOT where a pong
// puts it - a pong carries the server GUID first.
const pingMagicOffset = 9

// isOfflinePing reports whether data is a ping a real server would answer.
//
// The magic is checked, not just the packet ID, because that is what RakLib
// (PocketMine) and Nukkit do: they compare the magic and silently drop anything
// that fails. Being permissive here would let phantom send a malformed probe
// and still look healthy in every test.
func isOfflinePing(data []byte) bool {
	if len(data) < pingMagicOffset+len(corpus.Magic) {
		return false
	}
	if data[0] != corpus.PingID && data[0] != corpus.PingOpenID {
		return false
	}
	return bytes.Equal(data[pingMagicOffset:pingMagicOffset+len(corpus.Magic)], corpus.Magic)
}

// pongFor returns the configured pong with the caller's ping time stamped into
// it, which is what every real RakNet server does.
func (s *Server) pongFor(ping []byte) []byte {
	out := append([]byte(nil), s.opts.Pong...)
	if len(ping) >= 9 && len(out) >= 9 {
		copy(out[1:9], ping[1:9])
	}
	return out
}

// Stop makes the server go dark without unbinding, simulating an upstream that
// has crashed or stopped responding. Resume brings it back.
func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
}

// Resume undoes Stop.
func (s *Server) Resume() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = false
}

// Pings is the number of offline pings received.
func (s *Server) Pings() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pings
}

// Received returns copies of every datagram the server has seen.
func (s *Server) Received() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]byte, len(s.received))
	copy(out, s.received)
	return out
}

// LastPayload returns the most recent non-ping datagram, or nil.
func (s *Server) LastPayload() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.received) - 1; i >= 0; i-- {
		if d := s.received[i]; len(d) > 0 && !isOfflinePing(d) {
			return d
		}
	}
	return nil
}

// Close shuts the server down.
func (s *Server) Close() error {
	select {
	case <-s.done:
		return nil
	default:
		close(s.done)
	}
	err := s.conn.Close()
	s.wg.Wait()
	return err
}

// String aids test failure messages.
func (s *Server) String() string {
	return fmt.Sprintf("fakeserver(%s)", s.Addr())
}
