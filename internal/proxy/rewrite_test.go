package proxy

// T0 - white-box unit tests for the pong rewrite.
//
// This file lives in `package proxy` rather than in test/compat because
// rewriteUnconnectedPong is an unexported method; Go offers no way to reach it
// from an external test package. That is the constraint that splits T0 across
// internal/proto and internal/proxy. See test/compat/DESIGN.md section 3.
//
// The rewrite contract (DESIGN.md section 4): phantom replaces the server ID
// and the advertised ports, and must leave every other field exactly as the
// upstream server sent it.

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/jhead/phantom/internal/corpus"
)

const testBoundPort = 54321

// MOTD field indices, in PongData declaration order.
const (
	idxEdition = iota
	idxMOTD
	idxProtocolVersion
	idxVersion
	idxPlayers
	idxMaxPlayers
	idxServerID
	idxSubMOTD
	idxGameType
	idxGameMode // struct calls this NintendoLimited; it is really a numeric game mode
	idxPort4
	idxPort6
)

// newTestProxy builds the minimal ProxyServer the rewrite path actually touches.
// No sockets are opened - this is a pure-function test of packet surgery.
func newTestProxy(removePorts bool) *ProxyServer {
	return &ProxyServer{
		boundPort: testBoundPort,
		prefs:     ProxyPrefs{RemovePorts: removePorts},
	}
}

func fields(motd string) []string {
	parts := strings.Split(motd, ";")
	if n := len(parts); n > 0 && parts[n-1] == "" {
		parts = parts[:n-1]
	}
	return parts
}

// rewriteFields runs the rewrite and returns the incoming and outgoing MOTD fields.
func rewriteFields(t *testing.T, p *ProxyServer, frame []byte) (in, out []string, outFrame []byte) {
	t.Helper()

	inMOTD, err := corpus.SplitMOTD(frame)
	if err != nil {
		t.Fatalf("test fixture is not a well-formed pong: %v", err)
	}

	outFrame = p.rewriteUnconnectedPong(frame)

	outMOTD, err := corpus.SplitMOTD(outFrame)
	if err != nil {
		t.Fatalf("rewrite produced an unparseable pong: %v", err)
	}
	return fields(inMOTD), fields(outMOTD), outFrame
}

// TestRewritePreservesUpstreamFields is invariant 1: everything except the
// server ID and the two port fields must survive the rewrite untouched, across
// every captured protocol version.
func TestRewritePreservesUpstreamFields(t *testing.T) {
	rewritten := map[int]bool{idxServerID: true, idxPort4: true, idxPort6: true}

	for _, e := range corpus.Captured() {
		t.Run(e.MC, func(t *testing.T) {
			in, out, _ := rewriteFields(t, newTestProxy(false), e.Frame())

			for i := range in {
				if rewritten[i] || i >= len(out) {
					continue
				}
				if in[i] != out[i] {
					t.Errorf("field %d must pass through unchanged: %q -> %q", i, in[i], out[i])
				}
			}
		})
	}
}

// TestRewriteReplacesServerID is invariant 4a.
func TestRewriteReplacesServerID(t *testing.T) {
	want := fmt.Sprintf("%d", serverID)

	for _, e := range corpus.Captured() {
		t.Run(e.MC, func(t *testing.T) {
			in, out, _ := rewriteFields(t, newTestProxy(false), e.Frame())

			if out[idxServerID] != want {
				t.Errorf("ServerID = %q, want phantom instance id %q", out[idxServerID], want)
			}
			if in[idxServerID] == out[idxServerID] {
				t.Errorf("ServerID was not rewritten (upstream and output both %q); "+
					"restarting phantom would confuse clients", in[idxServerID])
			}
		})
	}
}

// TestRewriteRewritesPorts is invariant 5. Note the asymmetry: a server that
// advertises no ports keeps advertising none - phantom only rewrites ports that
// were already present.
func TestRewriteRewritesPorts(t *testing.T) {
	wantPort := fmt.Sprintf("%d", testBoundPort)

	t.Run("ports-present", func(t *testing.T) {
		frame := corpus.BuildPong(corpus.SyntheticPingTime(), corpus.SyntheticGUID(),
			"MCPE;Ports;800;1.21.80;0;10;12345678;Sub;Survival;1;19132;19133;")

		_, out, _ := rewriteFields(t, newTestProxy(false), frame)

		if out[idxPort4] != wantPort {
			t.Errorf("Port4 = %q, want %q", out[idxPort4], wantPort)
		}
		if out[idxPort6] != wantPort {
			t.Errorf("Port6 = %q, want %q", out[idxPort6], wantPort)
		}
	})

	t.Run("remove-ports-flag", func(t *testing.T) {
		frame := corpus.BuildPong(corpus.SyntheticPingTime(), corpus.SyntheticGUID(),
			"MCPE;Ports;800;1.21.80;0;10;12345678;Sub;Survival;1;19132;19133;")

		_, out, _ := rewriteFields(t, newTestProxy(true), frame)

		if len(out) > idxPort4 && out[idxPort4] != "" {
			t.Errorf("Port4 = %q, want empty under -remove_ports", out[idxPort4])
		}
		if len(out) > idxPort6 && out[idxPort6] != "" {
			t.Errorf("Port6 = %q, want empty under -remove_ports", out[idxPort6])
		}
	})

	t.Run("legacy-pong-without-ports", func(t *testing.T) {
		frame := corpus.BuildPong(corpus.SyntheticPingTime(), corpus.SyntheticGUID(),
			"MCPE;Legacy;390;1.14.60;0;10;12345678;Sub;Survival;1;")

		_, out, _ := rewriteFields(t, newTestProxy(false), frame)

		if len(out) > idxPort4 && out[idxPort4] != "" {
			t.Errorf("Port4 = %q; a server advertising no ports must keep advertising none",
				out[idxPort4])
		}
	})
}

// TestRewritePreservesTrailingFields is invariant 2 - the headline
// forwards-compatibility bug, exercised against real captured bytes.
func TestRewritePreservesTrailingFields(t *testing.T) {
	for _, e := range corpus.Captured() {
		t.Run(e.MC, func(t *testing.T) {
			in, out, _ := rewriteFields(t, newTestProxy(false), e.Frame())

			corpus.Check(t, "t0/trailing-fields-preserved", func() error {
				if len(out) < len(in) {
					return corpus.Errorf(
						"rewrite dropped %d trailing field(s) %q (upstream sent %d, phantom emitted %d)",
						len(in)-len(out), in[len(out):], len(in), len(out))
				}
				return nil
			})
		})
	}
}

// TestRewritePreservesHeader is invariant 3, plus the ping-time half of
// invariant 6: the rewrite must not disturb the RakNet header it passes through.
func TestRewritePreservesHeader(t *testing.T) {
	for _, e := range corpus.Captured() {
		t.Run(e.MC, func(t *testing.T) {
			frame := e.Frame()
			out := newTestProxy(false).rewriteUnconnectedPong(frame)

			if len(out) < 33 {
				t.Fatalf("rewritten packet is only %d bytes", len(out))
			}
			if !bytes.Equal(out[1:9], frame[1:9]) {
				t.Errorf("ping time changed: %x -> %x", frame[1:9], out[1:9])
			}
			if !bytes.Equal(out[17:33], corpus.Magic) {
				t.Errorf("RakNet magic corrupted: %x", out[17:33])
			}
		})
	}
}

// TestRewriteRewritesBinaryGUID is invariant 7. RakNet identifies a server by
// the 8-byte GUID at bytes 9-16, not by the MOTD string field. phantom rewrites
// only the string, so two instances proxying one upstream broadcast identical
// binary GUIDs.
func TestRewriteRewritesBinaryGUID(t *testing.T) {
	for _, e := range corpus.Captured() {
		t.Run(e.MC, func(t *testing.T) {
			frame := e.Frame()
			out := newTestProxy(false).rewriteUnconnectedPong(frame)

			corpus.Check(t, "t0/binary-guid-rewritten", func() error {
				if bytes.Equal(out[9:17], frame[9:17]) {
					return corpus.Errorf(
						"binary ServerGUID passed through unchanged (%x); "+
							"it must carry phantom's per-instance id, like the string ServerID does",
						out[9:17])
				}
				return nil
			})
		})
	}
}

// TestRewriteSurvivesMalformedInput: the rewrite runs on whatever arrives from
// the network. It must never panic, and on unparseable input it must fall back
// to forwarding the original bytes rather than dropping the packet.
func TestRewriteSurvivesMalformedInput(t *testing.T) {
	p := newTestProxy(false)

	for _, e := range corpus.Synthetic() {
		t.Run(e.ID, func(t *testing.T) {
			frame := e.Frame()

			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("PANIC rewriting %q: %v", e.ID, r)
				}
			}()

			out := p.rewriteUnconnectedPong(frame)
			if out == nil {
				t.Error("rewrite returned nil; the packet would be silently dropped")
			}
		})
	}
}
