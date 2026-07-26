package proto_test

// Unit tests over the cross-version pong corpus.
//
// These replay real captured wire bytes (51 Minecraft versions) and
// hand-authored shape fixtures through the parser and rebuilder, asserting the
// field-preservation contract in test/compat/DESIGN.md.
//
// No network, no subprocess, no Node. Runs everywhere in well under a second.

import (
	"bytes"
	"fmt"
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
			return fmt.Errorf(
				"field count dropped from %d to %d; lost trailing fields %q\n  in:  %q\n  out: %q",
				len(want), len(got), want[len(got):], in, out)
		}
		return fmt.Errorf(
			"field count grew from %d to %d\n  in:  %q\n  out: %q",
			len(want), len(got), in, out)
	}

	for i := range want {
		if want[i] != got[i] {
			return fmt.Errorf("field %d changed: %q -> %q\n  in:  %q\n  out: %q",
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
// version, measured against real captured bytes.
func TestCapturedCorpusRoundTrip(t *testing.T) {
	for _, e := range corpus.Captured() {
		t.Run(e.MC, func(t *testing.T) {
			out, err := roundTrip(e.Frame())
			if err != nil {
				t.Fatalf("round-trip failed: %v", err)
			}
			if err := assertFieldsPreserved(e.MOTD, out); err != nil {
				t.Error(err)
			}
		})
	}
}

// TestCapturedCorpusEchoesPingTime validates the corpus itself: every real
// server echoed the exact ping time we sent. This is the behaviour phantom's
// offline pong fails to reproduce (see e2e/offline-pong-echoes-ping-time), so
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

			if err := assertFieldsPreserved(e.MOTDString(), out); err != nil {
				t.Error(err)
			}
		})
	}
}

// TestMalformedFrames asserts the parser rejects input it cannot trust.
// Some currently parse into zero-padded garbage instead of erroring.
func TestMalformedFrames(t *testing.T) {
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

			if err == nil {
				t.Error("parser accepted malformed input that it should reject")
			}
		})
	}
}
