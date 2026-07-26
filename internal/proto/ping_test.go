package proto_test

// Unit tests for the Unconnected Ping side of the offline protocol.
//
// A ping is not a pong with the fields shuffled: RakNet puts the magic before
// the client GUID in a ping and after the server GUID in a pong. Getting that
// backwards produces 33 bytes that look plausible in a hex dump and that
// permissive servers still answer, so the mistake only shows up against
// implementations that validate the magic (RakLib/PocketMine, Nukkit). These
// tests pin the byte offsets so it cannot drift back.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jhead/phantom/internal/corpus"
	"github.com/jhead/phantom/internal/proto"
)

func TestBuildUnconnectedPingLayout(t *testing.T) {
	pingTime := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	guid := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x11, 0x22}

	ping := proto.BuildUnconnectedPing(pingTime, guid)

	if len(ping) != proto.UnconnectedPingLen {
		t.Fatalf("ping is %d bytes, want %d", len(ping), proto.UnconnectedPingLen)
	}
	if ping[0] != proto.UnconnectedPingID {
		t.Fatalf("packet ID = 0x%02x, want 0x%02x", ping[0], proto.UnconnectedPingID)
	}
	if !bytes.Equal(ping[1:9], pingTime) {
		t.Fatalf("ping time at [1:9] = %v, want %v", ping[1:9], pingTime)
	}
	if !bytes.Equal(ping[9:25], corpus.Magic) {
		t.Fatalf("magic must sit at [9:25] (before the GUID, unlike a pong); got %v", ping[9:25])
	}
	if !bytes.Equal(ping[25:33], guid) {
		t.Fatalf("client GUID at [25:33] = %v, want %v", ping[25:33], guid)
	}
}

func TestBuildUnconnectedPingPadsShortInputs(t *testing.T) {
	ping := proto.BuildUnconnectedPing(nil, nil)

	if len(ping) != proto.UnconnectedPingLen {
		t.Fatalf("ping is %d bytes, want %d", len(ping), proto.UnconnectedPingLen)
	}
	if !proto.IsUnconnectedPing(ping) {
		t.Fatal("a ping built from empty inputs must still be well-formed")
	}

	long := proto.BuildUnconnectedPing(make([]byte, 32), make([]byte, 32))
	if len(long) != proto.UnconnectedPingLen {
		t.Fatalf("over-long inputs produced %d bytes, want %d", len(long), proto.UnconnectedPingLen)
	}
}

func TestIsUnconnectedPing(t *testing.T) {
	valid := proto.BuildUnconnectedPing([]byte{9, 9, 9, 9, 9, 9, 9, 9}, nil)

	openConnections := append([]byte(nil), valid...)
	openConnections[0] = proto.UnconnectedPingOpenID

	// Some clients omit the trailing client GUID. The magic is all phantom
	// needs to recognise the packet, so a ping that stops after it is accepted.
	noGUID := append([]byte(nil), valid[:25]...)

	badMagic := append([]byte(nil), valid...)
	badMagic[9] ^= 0xFF

	// The exact malformation this whole file exists to catch: pong field order
	// applied to a ping, so the magic lands 8 bytes late.
	pongOrdered := make([]byte, 0, proto.UnconnectedPingLen)
	pongOrdered = append(pongOrdered, proto.UnconnectedPingID)
	pongOrdered = append(pongOrdered, make([]byte, 8)...) // ping time
	pongOrdered = append(pongOrdered, make([]byte, 8)...) // GUID, wrongly early
	pongOrdered = append(pongOrdered, corpus.Magic...)

	cases := []struct {
		name string
		in   []byte
		want bool
	}{
		{"well-formed 0x01", valid, true},
		{"well-formed 0x02", openConnections, true},
		{"magic present, GUID omitted", noGUID, true},
		{"corrupted magic", badMagic, false},
		{"pong field order", pongOrdered, false},
		{"truncated inside magic", valid[:24], false},
		{"pong packet id", append([]byte{proto.UnconnectedPongID}, valid[1:]...), false},
		{"empty", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := proto.IsUnconnectedPing(tc.in); got != tc.want {
				t.Fatalf("IsUnconnectedPing = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestBuildClampsOversizedMOTD covers the uint16 length prefix. A MOTD past
// 65535 bytes used to wrap the field, declaring a length that did not match the
// bytes that followed - which every parser reads as a corrupt pong.
func TestBuildClampsOversizedMOTD(t *testing.T) {
	packet := proto.UnconnectedPing{
		PingTime: make([]byte, 8),
		ID:       make([]byte, 8),
		Magic:    corpus.Magic,
		Pong: proto.PongData{
			Edition: "MCPE",
			MOTD:    strings.Repeat("A", 70000),
		},
	}

	built := packet.Build()
	frame := built.Bytes()

	motd, err := corpus.SplitMOTD(frame)
	if err != nil {
		t.Fatalf("oversized MOTD produced an unparseable pong: %v", err)
	}
	if want := len(frame) - 35; len(motd) != want {
		t.Fatalf("declared length %d does not match the %d bytes that follow", len(motd), want)
	}
	if len(motd) != 0xFFFF {
		t.Fatalf("MOTD clamped to %d bytes, want 65535", len(motd))
	}
}
