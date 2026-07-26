package fakeserver

// The fake upstream is a test double, but the thing it doubles is a RakNet
// server, and one of its jobs is to be as picky as a real one. RakLib
// (PocketMine) and Nukkit validate the offline magic and silently drop pings
// that fail. A fake that answered anything starting with 0x01 let phantom ship
// a probe with pong field order: every test passed, and every real
// magic-validating server ignored the probe.

import (
	"bytes"
	"net"
	"testing"
	"time"

	"github.com/jhead/phantom/internal/corpus"
)

func dial(t *testing.T, addr string) (*net.UDPConn, error) {
	t.Helper()
	remote, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	return net.DialUDP("udp", nil, remote)
}

// recvWithin returns the next datagram, or nil if none arrives. The window has
// to outlast the fake's 100ms read poll for a negative result to mean anything.
func recvWithin(c *net.UDPConn) []byte {
	_ = c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 2048)
	n, err := c.Read(buf)
	if err != nil {
		return nil
	}
	return buf[:n]
}

func wellFormedPing(id byte) []byte {
	out := make([]byte, 0, 33)
	out = append(out, id)
	out = append(out, 1, 2, 3, 4, 5, 6, 7, 8) // ping time
	out = append(out, corpus.Magic...)
	out = append(out, 9, 9, 9, 9, 9, 9, 9, 9) // client GUID
	return out
}

func TestIsOfflinePing(t *testing.T) {
	// The magic 8 bytes late, which is what applying pong field order to a
	// ping produces. Same length, same packet ID, still not a ping.
	pongOrdered := make([]byte, 0, 33)
	pongOrdered = append(pongOrdered, corpus.PingID)
	pongOrdered = append(pongOrdered, 1, 2, 3, 4, 5, 6, 7, 8)
	pongOrdered = append(pongOrdered, 9, 9, 9, 9, 9, 9, 9, 9)
	pongOrdered = append(pongOrdered, corpus.Magic...)

	corrupted := wellFormedPing(corpus.PingID)
	corrupted[12] ^= 0xFF

	// A RakNet data datagram that happens to start with a ping-like byte must
	// stay a payload, not become a ping.
	dataDatagram := make([]byte, 1400)
	dataDatagram[0] = corpus.PingID

	cases := []struct {
		name string
		in   []byte
		want bool
	}{
		{"0x01 ping", wellFormedPing(corpus.PingID), true},
		{"0x02 ping", wellFormedPing(corpus.PingOpenID), true},
		{"pong field order", pongOrdered, false},
		{"corrupted magic", corrupted, false},
		{"truncated before magic ends", wellFormedPing(corpus.PingID)[:20], false},
		{"payload with ping-like first byte", dataDatagram, false},
		{"pong", []byte{corpus.PongID}, false},
		{"empty", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isOfflinePing(tc.in); got != tc.want {
				t.Fatalf("isOfflinePing = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestServerAnswersOnlyWellFormedPings(t *testing.T) {
	s, err := Start(Opts{MOTD: "MCPE;Strict;800;1.21.80;0;10;123;Sub;Survival;1;19132;19133;"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	c, err := dial(t, s.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	pongOrdered := make([]byte, 0, 33)
	pongOrdered = append(pongOrdered, corpus.PingID)
	pongOrdered = append(pongOrdered, make([]byte, 8)...)
	pongOrdered = append(pongOrdered, make([]byte, 8)...)
	pongOrdered = append(pongOrdered, corpus.Magic...)

	if _, err := c.Write(pongOrdered); err != nil {
		t.Fatal(err)
	}
	if reply := recvWithin(c); reply != nil {
		t.Fatalf("a ping with the magic in the wrong place was answered with %d bytes", len(reply))
	}
	if got := s.Pings(); got != 0 {
		t.Fatalf("malformed ping counted as %d pings", got)
	}

	ping := wellFormedPing(corpus.PingID)
	if _, err := c.Write(ping); err != nil {
		t.Fatal(err)
	}
	reply := recvWithin(c)
	if reply == nil {
		t.Fatal("well-formed ping went unanswered")
	}
	if reply[0] != corpus.PongID {
		t.Fatalf("reply packet ID 0x%02x, want 0x%02x", reply[0], corpus.PongID)
	}
	if !bytes.Equal(reply[1:9], ping[1:9]) {
		t.Fatalf("reply ping time %x, want the echoed %x", reply[1:9], ping[1:9])
	}
	if got := s.Pings(); got != 1 {
		t.Fatalf("Pings() = %d, want 1", got)
	}
}
