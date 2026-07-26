package proto_test

// T0 - fuzzing.
//
// ReadUnconnectedPing parses untrusted bytes straight off a UDP socket with no
// length or magic validation, which makes it the highest-value fuzz target in
// the codebase. The corpus seeds it with real per-version pongs and every
// hand-authored malformed frame, so the fuzzer starts from realistic structure
// rather than having to discover the RakNet header from scratch.
//
// CI runs a short pass; the nightly workflow runs a long one.

import (
	"testing"

	"github.com/jhead/phantom/internal/corpus"
	"github.com/jhead/phantom/internal/proto"
)

func FuzzReadUnconnectedPing(f *testing.F) {
	for _, e := range corpus.Captured() {
		f.Add(e.Frame())
	}
	for _, e := range corpus.Synthetic() {
		f.Add(e.Frame())
	}
	// A few degenerate seeds the corpus does not contain.
	f.Add([]byte{})
	f.Add([]byte{corpus.PongID})
	f.Add(make([]byte, 1472)) // all zeros, max MTU

	f.Fuzz(func(t *testing.T, data []byte) {
		pkt, err := proto.ReadUnconnectedPing(data)
		if err != nil {
			return // rejecting input is always acceptable
		}
		if pkt == nil {
			t.Fatal("nil packet returned with nil error")
		}

		// Anything the parser accepted must also be rebuildable: phantom calls
		// Build() on every pong it forwards, so a parse/build asymmetry here is
		// a crash in the proxy's hot path.
		out := pkt.Build()

		body := out.Bytes()
		if len(body) == 0 {
			t.Fatal("Build() produced an empty packet")
		}
		if body[0] != corpus.PongID {
			t.Fatalf("Build() wrote packet id 0x%02x, want 0x%02x", body[0], corpus.PongID)
		}

		// The rebuilt packet must itself be parseable. If it is not, phantom
		// would be emitting pongs that no client can read.
		if _, err := proto.ReadUnconnectedPing(body); err != nil {
			t.Fatalf("Build() output is not re-parseable: %v", err)
		}
	})
}
