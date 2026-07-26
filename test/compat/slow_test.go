//go:build e2e && slow

package compat

// T1 - slow black-box cases.
//
// These are behind an extra `slow` tag because they wait on phantom's real
// timers: the idle sweep runs every 5s and cannot be shortened from outside the
// process. CI includes them; `make test` does not.
//
// Run with: go test -tags='e2e slow' ./test/compat/...

import (
	"strings"
	"testing"
	"time"

	"github.com/jhead/phantom/test/compat/fakeserver"
)

// TestIdleClientIsReaped is invariant 12. phantom sweeps for idle clients every
// 5 seconds, so with -timeout 1 a silent client should be dropped within ~6s.
func TestIdleClientIsReaped(t *testing.T) {
	srv, p := startPair(t,
		fakeserver.Opts{EchoPayloads: true},
		Opts{IdleTimeout: time.Second},
	)

	c, err := NewClient(p.Addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	// Establish a session.
	payload := []byte{0x84, 0x01, 0x02, 0x03}
	if err := c.Send(payload); err != nil {
		t.Fatalf("send: %v", err)
	}
	if _, err := c.Recv(PingTimeout); err != nil {
		t.Fatalf("session was never established: %v", err)
	}

	before := len(srv.Received())

	// Go quiet for longer than the idle timeout plus one sweep interval.
	time.Sleep(8 * time.Second)

	// The connection should have been reaped. Sending again must still work -
	// phantom is expected to build a fresh upstream connection, not to wedge.
	if err := c.Send(payload); err != nil {
		t.Fatalf("send after idle: %v", err)
	}
	if _, err := c.Recv(PingTimeout); err != nil {
		t.Fatalf("phantom stopped proxying after reaping an idle client: %v\n"+
			"--- phantom output ---\n%s", err, p.Output())
	}

	if len(srv.Received()) <= before {
		t.Error("upstream saw no new traffic after the idle period")
	}
}

// TestUpstreamRecovery is invariant 13: phantom must notice the upstream coming
// back and resume proxying without a restart.
func TestUpstreamRecovery(t *testing.T) {
	const motd = "MCPE;Recovery;800;1.21.80;0;10;123;Sub;Creative;1;19132;19133;0;"

	srv, p := startPair(t, fakeserver.Opts{MOTD: motd}, Opts{})

	// 1. Healthy: the upstream's MOTD reaches the client.
	pong, err := p.Ping()
	if err != nil {
		t.Fatalf("initial ping: %v", err)
	}
	if got := motdOf(t, pong); !strings.Contains(got, "Recovery") {
		t.Fatalf("expected the upstream MOTD, got %q", got)
	}

	// 2. Upstream goes dark: phantom should fall back to its offline pong.
	srv.Stop()
	if waitForOfflinePong(t, p, 0x01, DefaultPingTime()) == nil {
		t.Fatalf("phantom never fell back to an offline pong\n--- output ---\n%s", p.Output())
	}

	// 3. Upstream returns: phantom must resume proxying the real MOTD.
	srv.Resume()

	deadline := time.Now().Add(ReadyTimeout)
	for time.Now().Before(deadline) {
		pong, err := p.Ping()
		if err == nil && strings.Contains(motdOf(t, pong), "Recovery") {
			return
		}
		time.Sleep(pollInterval)
	}

	t.Fatalf("phantom did not resume proxying after the upstream returned\n"+
		"--- output ---\n%s", p.Output())
}
