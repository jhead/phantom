//go:build e2e

package compat

// End-to-end tests.
//
// Everything here drives a real phantom subprocess over real UDP sockets. No
// knowledge of phantom's internals is used or permitted; TestBlackBoxRuleHolds
// enforces that mechanically.
//
// Run with: go test -tags=e2e ./test/compat/...

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jhead/phantom/internal/corpus"
	"github.com/jhead/phantom/test/compat/fakeserver"
)

// fieldsOf splits a MOTD, dropping the empty element left by the conventional
// trailing semicolon.
func fieldsOf(motd string) []string {
	parts := strings.Split(motd, ";")
	if n := len(parts); n > 0 && parts[n-1] == "" {
		parts = parts[:n-1]
	}
	return parts
}

// motdOf extracts the MOTD from a pong datagram.
func motdOf(t *testing.T, pong []byte) string {
	t.Helper()
	motd, err := corpus.SplitMOTD(pong)
	if err != nil {
		t.Fatalf("phantom emitted an unparseable pong (%d bytes): %v", len(pong), err)
	}
	return motd
}

// startPair boots a fake upstream and a phantom pointed at it.
func startPair(t *testing.T, up fakeserver.Opts, po Opts) (*fakeserver.Server, *Phantom) {
	t.Helper()
	RequirePingPort(t)

	srv, err := fakeserver.Start(up)
	if err != nil {
		t.Fatalf("starting fake upstream: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	po.RemoteServer = srv.Addr()
	return srv, Start(t, po)
}

// TestPongPassesThroughUpstreamFields is invariant 1: everything except the
// server id and the ports must reach the client exactly as the upstream sent it.
func TestPongPassesThroughUpstreamFields(t *testing.T) {
	const motd = "MCPE;Passthrough Test;800;1.21.80;7;20;999888777;My Sub;Creative;1;19132;19133;0;"

	_, p := startPair(t, fakeserver.Opts{MOTD: motd}, Opts{})

	pong, err := p.Ping()
	if err != nil {
		t.Fatalf("ping: %v", err)
	}

	in, out := fieldsOf(motd), fieldsOf(motdOf(t, pong))

	// Indices 6 (server id), 10 and 11 (ports) are phantom's to rewrite.
	rewritten := map[int]bool{6: true, 10: true, 11: true}
	for i := range in {
		if rewritten[i] || i >= len(out) {
			continue
		}
		if in[i] != out[i] {
			t.Errorf("field %d must pass through unchanged: upstream %q, client saw %q",
				i, in[i], out[i])
		}
	}
}

// TestTrailingFieldsReachClient is invariant 2 measured end-to-end: a real
// client pinging through phantom never sees the 13th field.
func TestTrailingFieldsReachClient(t *testing.T) {
	const motd = "MCPE;Trailing;800;1.21.80;0;10;123;Sub;Creative;1;19132;19133;0;"

	_, p := startPair(t, fakeserver.Opts{MOTD: motd}, Opts{})

	pong, err := p.Ping()
	if err != nil {
		t.Fatalf("ping: %v", err)
	}

	in, out := fieldsOf(motd), fieldsOf(motdOf(t, pong))

	if len(out) < len(in) {
		t.Errorf("client received %d fields, upstream sent %d; lost %q",
			len(out), len(in), in[len(out):])
	}
}

// TestServerIDIsStableAndPerInstance is invariant 4b, and the reason phantom is
// run as a subprocess: serverID is a package global, so two instances cannot be
// distinguished from inside one process.
func TestServerIDIsStableAndPerInstance(t *testing.T) {
	const motd = "MCPE;ID Test;800;1.21.80;0;10;111222333;Sub;Creative;1;19132;19133;0;"

	srv, p1 := startPair(t, fakeserver.Opts{MOTD: motd}, Opts{})

	idOf := func(p *Phantom) string {
		t.Helper()
		pong, err := p.Ping()
		if err != nil {
			t.Fatalf("ping: %v", err)
		}
		f := fieldsOf(motdOf(t, pong))
		if len(f) < 7 {
			t.Fatalf("pong has only %d fields", len(f))
		}
		return f[6]
	}

	first := idOf(p1)

	t.Run("stable-across-pings", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			if got := idOf(p1); got != first {
				t.Fatalf("server id changed between pings: %q then %q", first, got)
			}
		}
	})

	t.Run("differs-from-upstream", func(t *testing.T) {
		if first == "111222333" {
			t.Error("phantom forwarded the upstream's server id unchanged; " +
				"restarting phantom would confuse clients")
		}
	})

	t.Run("differs-between-instances", func(t *testing.T) {
		p2 := Start(t, Opts{RemoteServer: srv.Addr()})
		if second := idOf(p2); second == first {
			t.Errorf("two phantom instances advertised the same server id %q; "+
				"clients cannot tell them apart", second)
		}
	})
}

// TestPortRewriting is invariant 5, exercised through the real CLI flags.
func TestPortRewriting(t *testing.T) {
	const motd = "MCPE;Ports;800;1.21.80;0;10;123;Sub;Creative;1;19132;19133;0;"

	t.Run("rewritten-to-bind-port", func(t *testing.T) {
		_, p := startPair(t, fakeserver.Opts{MOTD: motd}, Opts{})

		pong, err := p.Ping()
		if err != nil {
			t.Fatalf("ping: %v", err)
		}
		f := fieldsOf(motdOf(t, pong))
		want := fmt.Sprintf("%d", p.BindPort)

		if f[10] != want {
			t.Errorf("Port4 = %q, want phantom's bind port %q", f[10], want)
		}
		if f[11] != want {
			t.Errorf("Port6 = %q, want phantom's bind port %q", f[11], want)
		}
	})

	t.Run("remove_ports-flag", func(t *testing.T) {
		_, p := startPair(t, fakeserver.Opts{MOTD: motd}, Opts{RemovePorts: true})

		pong, err := p.Ping()
		if err != nil {
			t.Fatalf("ping: %v", err)
		}
		f := fieldsOf(motdOf(t, pong))

		if len(f) > 10 && f[10] != "" {
			t.Errorf("Port4 = %q, want empty under -remove_ports", f[10])
		}
		if len(f) > 11 && f[11] != "" {
			t.Errorf("Port6 = %q, want empty under -remove_ports", f[11])
		}
	})
}

// TestProxiedPongEchoesPingTime is the online half of invariant 6. The upstream
// stamps the client's ping time; phantom must not lose it in the rewrite.
func TestProxiedPongEchoesPingTime(t *testing.T) {
	_, p := startPair(t, fakeserver.Opts{}, Opts{})

	want := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x01, 0x02, 0x03, 0x04}
	pong, err := p.PingWith(corpus.PingID, want)
	if err != nil {
		t.Fatalf("ping: %v", err)
	}
	if len(pong) < 9 {
		t.Fatalf("pong too short: %d bytes", len(pong))
	}
	if !bytes.Equal(pong[1:9], want) {
		t.Errorf("pong echoed ping time %x, want %x", pong[1:9], want)
	}
}

// deadUpstream points phantom at a closed loopback port so it detects the
// upstream as offline (ECONNREFUSED arrives promptly on loopback).
func deadUpstream(t *testing.T) *Phantom {
	t.Helper()
	RequirePingPort(t)

	port := freeUDPPort(t)
	return Start(t, Opts{RemoteServer: fmt.Sprintf("127.0.0.1:%d", port)})
}

// negativeWait is the budget for confirming a pong will NOT arrive. It is much
// shorter than ReadyTimeout because the caller has already established that
// phantom is up and has detected the upstream as offline - so there is nothing
// left to wait for, and burning the full readiness budget on every negative
// case would dominate the suite's runtime.
const negativeWait = 3 * time.Second

// waitForOfflinePong pings until phantom answers with its canned offline pong.
func waitForOfflinePong(t *testing.T, p *Phantom, packetID byte, pingTime []byte) []byte {
	t.Helper()
	return waitForOfflinePongWithin(t, p, packetID, pingTime, ReadyTimeout)
}

func waitForOfflinePongWithin(t *testing.T, p *Phantom, packetID byte, pingTime []byte, budget time.Duration) []byte {
	t.Helper()

	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		pong, err := p.PingWith(packetID, pingTime)
		if err == nil && strings.Contains(strings.ToLower(motdOf(t, pong)), "offline") {
			return pong
		}
		time.Sleep(pollInterval)
	}
	return nil
}

// TestOfflinePong covers the server-offline path: invariants 6, 7 and 14.
func TestOfflinePong(t *testing.T) {
	t.Run("is-sent-when-upstream-is-down", func(t *testing.T) {
		p := deadUpstream(t)
		if waitForOfflinePong(t, p, corpus.PingID, DefaultPingTime()) == nil {
			t.Fatalf("phantom never sent an offline pong\n--- output ---\n%s", p.Output())
		}
	})

	t.Run("echoes-ping-time", func(t *testing.T) {
		p := deadUpstream(t)
		want := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0x11, 0x22, 0x33, 0x44}

		pong := waitForOfflinePong(t, p, corpus.PingID, want)
		if pong == nil {
			t.Fatalf("phantom never sent an offline pong\n--- output ---\n%s", p.Output())
		}

		if !bytes.Equal(pong[1:9], want) {
			t.Errorf(
				"offline pong carries ping time %x, want the client's %x; "+
					"the client uses this to compute displayed latency",
				pong[1:9], want)
		}
	})

	t.Run("carries-nonzero-guid", func(t *testing.T) {
		p := deadUpstream(t)

		pong := waitForOfflinePong(t, p, corpus.PingID, DefaultPingTime())
		if pong == nil {
			t.Fatalf("phantom never sent an offline pong\n--- output ---\n%s", p.Output())
		}

		if bytes.Equal(pong[9:17], make([]byte, 8)) {
			t.Errorf(
				"offline pong advertises binary ServerGUID 0; every phantom whose " +
					"upstream is down would claim the same RakNet identity")
		}
	})

	t.Run("answers-0x02-ping", func(t *testing.T) {
		p := deadUpstream(t)

		// Establish that the upstream really is detected as down, so a failure
		// below is about 0x02 handling and not about timing.
		if waitForOfflinePong(t, p, corpus.PingID, DefaultPingTime()) == nil {
			t.Fatalf("phantom never sent an offline pong for 0x01\n--- output ---\n%s", p.Output())
		}

		if waitForOfflinePongWithin(t, p, corpus.PingOpenID, DefaultPingTime(), negativeWait) == nil {
			t.Errorf(
				"a client pinging with 0x02 (Unconnected Ping Open Connections) " +
					"got no offline pong; the offline path matches only 0x01")
		}
	})
}

// TestPayloadIntegrity is invariant 10: non-ping datagrams must survive the
// round trip through phantom byte for byte, at every size RakNet can produce.
func TestPayloadIntegrity(t *testing.T) {
	_, p := startPair(t, fakeserver.Opts{EchoPayloads: true}, Opts{})

	// RakNet's ceiling is MTU 1492 minus IP (20) and UDP (8) headers = 1464.
	// phantom's read buffer is 1472, so 1464 and 1472 are the interesting edges.
	for _, size := range []int{1, 576, 1400, 1464, 1472} {
		t.Run(fmt.Sprintf("%d-bytes", size), func(t *testing.T) {
			c, err := NewClient(p.Addr)
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			defer c.Close()

			payload := make([]byte, size)
			payload[0] = 0x84 // a RakNet data datagram, not an offline ping
			for i := 1; i < size; i++ {
				payload[i] = byte(i % 251)
			}

			if err := c.Send(payload); err != nil {
				t.Fatalf("send: %v", err)
			}
			got, err := c.Recv(PingTimeout)
			if err != nil {
				t.Fatalf("no echo came back: %v", err)
			}
			if !bytes.Equal(got, payload) {
				t.Errorf("payload corrupted: sent %d bytes, got %d back", len(payload), len(got))
			}
		})
	}
}

// TestOversizedDatagramTruncates documents, rather than condemns, what happens
// past phantom's 1472-byte read buffer. RakNet cannot produce a datagram this
// large (its own ceiling is 1464), so this is unreachable in practice - but
// pinning the behaviour means a future buffer change is a deliberate decision
// rather than an accident.
func TestOversizedDatagramTruncates(t *testing.T) {
	srv, p := startPair(t, fakeserver.Opts{EchoPayloads: true}, Opts{})

	c, err := NewClient(p.Addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	payload := make([]byte, 1500)
	payload[0] = 0x84
	if err := c.Send(payload); err != nil {
		t.Fatalf("send: %v", err)
	}

	if _, err := c.Recv(PingTimeout); err != nil {
		t.Logf("no echo returned for an oversized datagram: %v", err)
	}

	if got := srv.LastPayload(); got != nil && len(got) == len(payload) {
		t.Errorf("upstream received all %d bytes; phantom's 1472-byte buffer "+
			"was expected to truncate. Buffer size may have changed.", len(got))
	}
}

// TestConcurrentClientsAreIsolated is invariant 11: two clients sharing one
// phantom must never receive each other's traffic.
func TestConcurrentClientsAreIsolated(t *testing.T) {
	_, p := startPair(t, fakeserver.Opts{EchoPayloads: true}, Opts{})

	const clients = 4
	type conn struct {
		c   *Client
		tag byte
	}

	var conns []conn
	for i := 0; i < clients; i++ {
		c, err := NewClient(p.Addr)
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		defer c.Close()
		conns = append(conns, conn{c: c, tag: byte(0x40 + i)})
	}

	// Each client sends a payload uniquely filled with its own tag.
	for _, cn := range conns {
		payload := bytes.Repeat([]byte{cn.tag}, 64)
		payload[0] = 0x84
		if err := cn.c.Send(payload); err != nil {
			t.Fatalf("send: %v", err)
		}
	}

	for i, cn := range conns {
		got, err := cn.c.Recv(PingTimeout)
		if err != nil {
			t.Fatalf("client %d got no echo: %v", i, err)
		}
		for j, b := range got[1:] {
			if b != cn.tag {
				t.Fatalf("client %d received another client's data at byte %d: "+
					"got 0x%02x, want 0x%02x", i, j+1, b, cn.tag)
			}
		}
	}
}

// TestBlackBoxRuleHolds enforces the tier boundary from DESIGN.md section 3.
// Without this, the black-box rule is a convention that decays the first time
// someone reaches for a convenient internal helper.
func TestBlackBoxRuleHolds(t *testing.T) {
	forbidden := []string{
		"github.com/jhead/phantom/internal/proto",
		"github.com/jhead/phantom/internal/proxy",
		"github.com/jhead/phantom/internal/clientmap",
	}

	root, err := repoRoot()
	if err != nil {
		t.Fatalf("locating repo root: %v", err)
	}

	cmd := exec.Command("go", "list", "-tags=e2e,slow", "-deps", "./test/compat/...")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}

	deps := string(out)
	for _, pkg := range forbidden {
		for _, line := range strings.Split(deps, "\n") {
			if strings.TrimSpace(line) == pkg {
				t.Errorf("test/compat imports %s.\n"+
					"the e2e suite may know only the phantom binary, its "+
					"CLI flags, and UDP. If an assertion needs phantom's internals, it "+
					"belongs in a unit test (internal/proto or internal/proxy).", pkg)
			}
		}
	}
}
