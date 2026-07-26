package proxy

import (
	"testing"

	"github.com/jhead/phantom/internal/proto"
)

// geyser-style MOTD: 10 fields, no Port4/Port6 (see GeyserMC advertisement format).
func geyserStylePong() []byte {
	pong := proto.UnconnectedPing{
		PingTime: []byte{0, 0, 0, 0, 0, 0, 0, 1},
		ID:       []byte{0, 0, 0, 0, 0, 0, 0, 2},
		Magic:    []byte{0x00, 0xff, 0xff, 0x00, 0xfe, 0xfe, 0xfe, 0xfe, 0xfd, 0xfd, 0xfd, 0xfd, 0x12, 0x34, 0x56, 0x78},
		Pong: proto.PongData{
			Edition:         "MCPE",
			MOTD:            "Geyser",
			ProtocolVersion: "557",
			Version:         "1.19.40",
			Players:         "0",
			MaxPlayers:      "20",
			ServerID:        "12345",
			SubMOTD:         "Geyser",
			GameType:        "Survival",
			NintendoLimited: "1",
			// Port4/Port6 intentionally empty — matches Geyser
		},
	}
	buf := pong.Build()
	return buf.Bytes()
}

func TestRewriteUnconnectedPongInjectsPortsWhenUpstreamOmitsThem(t *testing.T) {
	t.Parallel()

	p := &ProxyServer{
		boundPort: 55997,
		prefs:     ProxyPrefs{},
	}

	rewritten := p.rewriteUnconnectedPong(geyserStylePong())
	packet, err := proto.ReadUnconnectedPing(rewritten)
	if err != nil {
		t.Fatalf("parse rewritten pong: %v", err)
	}
	if packet.Pong.Port4 != "55997" {
		t.Fatalf("Port4 = %q, want 55997", packet.Pong.Port4)
	}
	if packet.Pong.Port6 != "55997" {
		t.Fatalf("Port6 = %q, want 55997", packet.Pong.Port6)
	}
}

func TestRewriteUnconnectedPongOverwritesExistingPorts(t *testing.T) {
	t.Parallel()

	p := &ProxyServer{
		boundPort: 19133,
		prefs:     ProxyPrefs{},
	}

	src := proto.UnconnectedPing{
		PingTime: []byte{0, 0, 0, 0, 0, 0, 0, 1},
		ID:       []byte{0, 0, 0, 0, 0, 0, 0, 2},
		Magic:    []byte{0x00, 0xff, 0xff, 0x00, 0xfe, 0xfe, 0xfe, 0xfe, 0xfd, 0xfd, 0xfd, 0xfd, 0x12, 0x34, 0x56, 0x78},
		Pong: proto.PongData{
			Edition:         "MCPE",
			MOTD:            "BDS",
			ProtocolVersion: "557",
			Version:         "1.19.40",
			Players:         "0",
			MaxPlayers:      "10",
			ServerID:        "99",
			SubMOTD:         "world",
			GameType:        "Survival",
			NintendoLimited: "0",
			Port4:           "19132",
			Port6:           "19132",
		},
	}

	buf := src.Build()
	rewritten := p.rewriteUnconnectedPong(buf.Bytes())
	packet, err := proto.ReadUnconnectedPing(rewritten)
	if err != nil {
		t.Fatalf("parse rewritten pong: %v", err)
	}
	if packet.Pong.Port4 != "19133" || packet.Pong.Port6 != "19133" {
		t.Fatalf("ports = %q/%q, want 19133/19133", packet.Pong.Port4, packet.Pong.Port6)
	}
}

func TestRewriteUnconnectedPongRemovePorts(t *testing.T) {
	t.Parallel()

	p := &ProxyServer{
		boundPort: 55997,
		prefs:     ProxyPrefs{RemovePorts: true},
	}

	src := proto.UnconnectedPing{
		PingTime: []byte{0, 0, 0, 0, 0, 0, 0, 1},
		ID:       []byte{0, 0, 0, 0, 0, 0, 0, 2},
		Magic:    []byte{0x00, 0xff, 0xff, 0x00, 0xfe, 0xfe, 0xfe, 0xfe, 0xfd, 0xfd, 0xfd, 0xfd, 0x12, 0x34, 0x56, 0x78},
		Pong: proto.PongData{
			Edition: "MCPE",
			MOTD:    "x",
			Port4:   "19132",
			Port6:   "19132",
		},
	}

	buf := src.Build()
	rewritten := p.rewriteUnconnectedPong(buf.Bytes())
	packet, err := proto.ReadUnconnectedPing(rewritten)
	if err != nil {
		t.Fatalf("parse rewritten pong: %v", err)
	}
	if packet.Pong.Port4 != "" || packet.Pong.Port6 != "" {
		t.Fatalf("expected empty ports, got %q/%q", packet.Pong.Port4, packet.Pong.Port6)
	}
}
