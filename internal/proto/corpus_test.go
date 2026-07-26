package proto_test

// T0 - white-box unit tests over the cross-version pong corpus.
//
// These replay real captured wire bytes (51 Minecraft versions) and
// hand-authored shape fixtures through the parser and rebuilder, asserting the
// field-preservation contract from test/compat/DESIGN.md section 4.
//
// No network, no subprocess, no Node. Runs everywhere in well under a second.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jhead/phantom/internal/corpus"
	"github.com/jhead/phantom/internal/proto"
)

// fieldsOf splits a MOTD, dropping the empty element produced by the
// conventional trailing semicolon. Comparing field *lists* rather than raw
// strings is deliberate: appending or omitting the trailing ';' is
// semantically meaningless to a client, so it must not count as a difference.
func fieldsOf(motd string) []string {
	parts := strings.Split(motd, ";")
	if n := len(parts); n > 0 && parts[n-1] == "" {
		parts = parts[:n-1]
	}
	return parts
}

// roundTrip parses a pong frame and rebuilds it, returning the resulting MOTD.
func roundTrip(frame []byte) (string, error) {
	pkt, err := proto.ReadUnconnectedPing(frame)
	if err != nil {
		return "", err
	}
	rebuilt := pkt.Build()
	return corpus.SplitMOTD(rebuilt.Bytes())
}

// assertFieldsPreserved is the core invariant: every MOTD field present on the
// wire must survive parse -> rebuild unchanged, including fields phantom has no
// struct member for.
func assertFieldsPreserved(in, out string) error {
	want, got := fieldsOf(in), fieldsOf(out)

	if len(want) != len(got) {
		if len(got) < len(want) {
			return corpus.Errorf(
				"field count dropped from %d to %d; lost trailing fields %q\n  in:  %q\n  out: %q",
				len(want), len(got), want[len(got):], in, out)
		}
		return corpus.Errorf(
			"field count grew from %d to %d\n  in:  %q\n  out: %q",
			len(want), len(got), in, out)
	}

	for i := range want {
		if want[i] != got[i] {
			return corpus.Errorf("field %d changed: %q -> %q\n  in:  %q\n  out: %q",
				i, want[i], got[i], in, out)
		}
	}
	return nil
}

// TestCapturedCorpusParses asserts every real captured pong parses cleanly and
// that the parser reads the version-identifying fields correctly. This is the
// broad sweep across all supported protocol versions.
func TestCapturedCorpusParses(t *testing.T) {
	entries := corpus.Captured()
	if len(entries) == 0 {
		t.Fatal("captured corpus is empty; run `make corpus`")
	}
	t.Logf("replaying %d captured versions from %s", len(entries), corpus.GeneratedBy())

	for _, e := range entries {
		t.Run(e.MC, func(t *testing.T) {
			pkt, err := proto.ReadUnconnectedPing(e.Frame())
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}

			want := fieldsOf(e.MOTD)
			if pkt.Pong.Edition != want[0] {
				t.Errorf("Edition = %q, want %q", pkt.Pong.Edition, want[0])
			}
			if pkt.Pong.ProtocolVersion != want[2] {
				t.Errorf("ProtocolVersion = %q, want %q", pkt.Pong.ProtocolVersion, want[2])
			}
			if pkt.Pong.Version != want[3] {
				t.Errorf("Version = %q, want %q", pkt.Pong.Version, want[3])
			}
			if !bytes.Equal(pkt.Magic, corpus.Magic) {
				t.Errorf("Magic = %x, want %x", pkt.Magic, corpus.Magic)
			}
		})
	}
}

// TestCapturedCorpusRoundTrip asserts field preservation across every captured
// version. Every real server sends 13 fields, so this currently XFAILs for all
// of them - that is the headline compatibility bug, measured against real bytes.
func TestCapturedCorpusRoundTrip(t *testing.T) {
	for _, e := range corpus.Captured() {
		t.Run(e.MC, func(t *testing.T) {
			out, err := roundTrip(e.Frame())
			if err != nil {
				t.Fatalf("round-trip failed: %v", err)
			}
			corpus.Check(t, "t0/trailing-fields-preserved", func() error {
				return assertFieldsPreserved(e.MOTD, out)
			})
		})
	}
}

// TestCapturedCorpusEchoesPingTime validates the corpus itself: every real
// server echoed the exact ping time we sent. This is the behaviour phantom's
// offline pong fails to reproduce (see t1/offline-pong-echoes-ping-time), so
// pinning it here documents the requirement with evidence.
func TestCapturedCorpusEchoesPingTime(t *testing.T) {
	sent := corpus.CapturedPingTime()

	for _, e := range corpus.Captured() {
		t.Run(e.MC, func(t *testing.T) {
			pkt, err := proto.ReadUnconnectedPing(e.Frame())
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if !bytes.Equal(pkt.PingTime, sent) {
				t.Errorf("server echoed ping time %x, want %x", pkt.PingTime, sent)
			}
		})
	}
}

// TestSyntheticShapes covers pong shapes the captured corpus cannot reach:
// historical field counts, hypothetical future ones, and hostile content.
func TestSyntheticShapes(t *testing.T) {
	for _, e := range corpus.Synthetic() {
		if e.IsRaw() || e.WantParseError {
			continue // rejection cases are covered by TestMalformedFrames
		}

		t.Run(e.ID, func(t *testing.T) {
			t.Log(e.Desc)

			out, err := roundTrip(e.Frame())
			if err != nil {
				t.Fatalf("round-trip failed: %v", err)
			}
			if e.NoRoundTrip {
				return
			}

			// Only fixtures carrying more fields than phantom's 12-field struct
			// are expected to lose data today.
			id := "t0/no-such-failure"
			if corpus.RealFieldCount(e.MOTDString()) > 12 {
				id = "t0/trailing-fields-preserved"
			}
			corpus.Check(t, id, func() error {
				return assertFieldsPreserved(e.MOTDString(), out)
			})
		})
	}
}

// TestMalformedFrames asserts the parser rejects input it cannot trust.
// Several of these currently parse into zero-padded garbage instead of
// erroring; each is registered against its TODO.md entry.
func TestMalformedFrames(t *testing.T) {
	// Which registered bug each malformed fixture demonstrates. Fixtures absent
	// from this map are expected to be rejected correctly today - phantom does
	// catch outright truncation, just not semantic defects like a bad magic.
	xfailByID := map[string]string{
		"malformed/length-exceeds-body": "t0/short-read-rejected",
		"malformed/bad-magic":           "t0/magic-validated",
		"malformed/wrong-packet-id":     "t0/packet-id-validated",
	}

	for _, e := range corpus.Synthetic() {
		if !e.IsRaw() && !e.WantParseError {
			continue
		}

		t.Run(e.ID, func(t *testing.T) {
			t.Log(e.Desc)

			// Must never panic, whatever the input.
			var err error
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("PANIC on malformed input: %v", r)
					}
				}()
				_, err = proto.ReadUnconnectedPing(e.Frame())
			}()

			if !e.WantParseError {
				if err != nil {
					t.Errorf("expected clean parse, got error: %v", err)
				}
				return
			}

			id, ok := xfailByID[e.ID]
			if !ok {
				id = "t0/no-such-failure"
			}
			corpus.Check(t, id, func() error {
				if err == nil {
					return corpus.Errorf("parser accepted malformed input that it should reject")
				}
				return nil
			})
		})
	}
}

// TestKnownFailureRegistryIsWellFormed guards the guard: a typo in an id would
// silently turn an XFAIL into an unreported pass.
func TestKnownFailureRegistryIsWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range corpus.KnownFailures() {
		if f.ID == "" {
			t.Error("known failure with empty id")
		}
		if f.Reason == "" {
			t.Errorf("known failure %q has no reason", f.ID)
		}
		if f.TODO == "" {
			t.Errorf("known failure %q has no TODO.md reference", f.ID)
		}
		if seen[f.ID] {
			t.Errorf("duplicate known failure id %q", f.ID)
		}
		seen[f.ID] = true
	}
}
