package proto

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/jhead/phantom/internal/corpus"
	"github.com/jhead/phantom/internal/util"
)

var UnconnectedPingID byte = 0x01
var UnconnectedPingOpenID byte = 0x02
var UnconnectedPongID byte = 0x1C

// IsUnconnectedDiscoveryPing reports whether id is an offline LAN discovery ping.
func IsUnconnectedDiscoveryPing(id byte) bool {
	return id == UnconnectedPingID || id == UnconnectedPingOpenID
}

// UnconnectedPingLen is the exact size of an Unconnected Ping:
// ID(1) + ping time(8) + magic(16) + client GUID(8).
const UnconnectedPingLen = 1 + 8 + 16 + 8

// pingMagicOffset is where the magic sits in an Unconnected Ping. Note this is
// NOT the same layout as an Unconnected Pong, which carries the server GUID
// *before* the magic. Getting the two confused produces a packet that RakNet
// implementations validating the magic (RakLib/PocketMine, Nukkit) silently drop.
const pingMagicOffset = 1 + 8

// IsUnconnectedPing reports whether data is a well-formed offline discovery
// ping: a known ping ID, the right length, and the RakNet magic in the position
// the ping layout puts it.
func IsUnconnectedPing(data []byte) bool {
	if len(data) < pingMagicOffset+len(corpus.Magic) {
		return false
	}
	if !IsUnconnectedDiscoveryPing(data[0]) {
		return false
	}
	return bytes.Equal(data[pingMagicOffset:pingMagicOffset+len(corpus.Magic)], corpus.Magic)
}

// BuildUnconnectedPing assembles a RakNet Unconnected Ping (0x01).
//
// Field order is ID, ping time, magic, client GUID — the magic precedes the
// GUID here, the reverse of Unconnected Pong. pingTime and clientGUID are
// zero-padded or truncated to 8 bytes.
func BuildUnconnectedPing(pingTime, clientGUID []byte) []byte {
	out := make([]byte, 0, UnconnectedPingLen)
	out = append(out, UnconnectedPingID)
	out = append(out, eightBytes(pingTime)...)
	out = append(out, corpus.Magic...)
	out = append(out, eightBytes(clientGUID)...)
	return out
}

func eightBytes(in []byte) []byte {
	var out [8]byte
	copy(out[:], in)
	return out[:]
}

type UnconnectedPing struct {
	PingTime []byte
	ID       []byte
	Magic    []byte
	Pong     PongData
}

// pongModeledFieldCount is the number of semicolon-separated MOTD fields
// phantom models explicitly. Servers may send additional trailing fields;
// those are stored in PongData.Extra and round-tripped unchanged.
const pongModeledFieldCount = 12

// maxPongDataLen is the largest MOTD the pong's uint16 length prefix can describe.
const maxPongDataLen = 0xFFFF

type PongData struct {
	Edition         string
	MOTD            string
	ProtocolVersion string
	Version         string
	Players         string
	MaxPlayers      string
	ServerID        string
	SubMOTD         string
	GameType        string
	NintendoLimited string
	Port4           string
	Port6           string
	Extra           []string
}

// OfflineMOTD is the server name phantom advertises while the remote is
// unreachable.
const OfflineMOTD = "phantom §cServer offline"

var OfflinePong = UnconnectedPing{
	PingTime: []byte{0, 0, 0, 0, 0, 0, 0, 0},
	ID:       []byte{0, 0, 0, 0, 0, 0, 0, 0},
	Magic:    []byte{0x00, 0xff, 0xff, 0x00, 0xfe, 0xfe, 0xfe, 0xfe, 0xfd, 0xfd, 0xfd, 0xfd, 0x12, 0x34, 0x56, 0x78},
	Pong: PongData{
		Edition: "MCPE",
		MOTD:    OfflineMOTD,
		// Cold-start only: once the remote has answered even once, the offline
		// advertisement is rebuilt from that server's own pong so the version it
		// claims matches the server behind it. This fallback covers the case
		// where phantom has never reached the remote, so it should track the
		// newest version in internal/corpus/data/captured.json — a stale value
		// here makes the offline entry unjoinable-looking to current clients.
		ProtocolVersion: "1001",
		Version:         "1.26.30",
		Players:         "0",
		// Non-zero capacity: some consoles omit full/zero-slot entries from Friends.
		MaxPlayers:      "1",
		GameType:        "Creative",
		NintendoLimited: "1",
		// Placeholder ports so rewriteUnconnectedPong overwrites them with
		// phantom's bind port. Empty ports make clients fall back to :19132,
		// whose replies come from the data port and fail the join (#104).
		Port4: "0",
		Port6: "0",
	},
}.Build()

var dupeSemicolonRegex = regexp.MustCompile(";{2,}$")

func ReadUnconnectedPing(in []byte) (*UnconnectedPing, error) {
	reply := &UnconnectedPing{}
	buf := bytes.NewReader(in)

	packetID, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}
	if packetID != UnconnectedPongID {
		return nil, fmt.Errorf("unexpected packet ID 0x%02x, want 0x%02x", packetID, UnconnectedPongID)
	}

	reply.PingTime = make([]byte, 8)
	if _, err := io.ReadFull(buf, reply.PingTime); err != nil {
		return nil, err
	}

	reply.ID = make([]byte, 8)
	if _, err := io.ReadFull(buf, reply.ID); err != nil {
		return nil, err
	}

	reply.Magic = make([]byte, len(corpus.Magic))
	if _, err := io.ReadFull(buf, reply.Magic); err != nil {
		return nil, err
	}
	if !bytes.Equal(reply.Magic, corpus.Magic) {
		return nil, fmt.Errorf("invalid RakNet magic")
	}

	pongLenBytes := make([]byte, 2)
	if _, err := io.ReadFull(buf, pongLenBytes); err != nil {
		return nil, err
	}

	pongLen := binary.BigEndian.Uint16(pongLenBytes)

	pongDataBytes := make([]byte, pongLen)
	if _, err := io.ReadFull(buf, pongDataBytes); err != nil {
		return nil, err
	}

	reply.Pong = readPong(string(pongDataBytes))

	return reply, nil
}

func (r UnconnectedPing) Build() bytes.Buffer {
	var outBuffer bytes.Buffer

	outBuffer.WriteByte(UnconnectedPongID)
	outBuffer.Write(r.PingTime)
	outBuffer.Write(r.ID)
	outBuffer.Write(r.Magic)

	pongDataString := writePong(r.Pong)

	// The wire length is a uint16. A longer MOTD would wrap the field and
	// declare a length that does not match the bytes that follow, which every
	// parser reads as a truncated or corrupt pong. Clamp instead.
	if len(pongDataString) > maxPongDataLen {
		pongDataString = pongDataString[:maxPongDataLen]
	}
	pongDataLen := len(pongDataString)

	stringBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(stringBuf, uint16(pongDataLen))

	outBuffer.Write(stringBuf)
	outBuffer.WriteString(pongDataString)

	return outBuffer
}

// Reads pong data from the string off the wire into an empty PongData struct
func readPong(raw string) PongData {
	pong := PongData{}

	stringParts := strings.Split(raw, ";")
	if n := len(stringParts); n > 0 && stringParts[n-1] == "" {
		stringParts = stringParts[:n-1]
	}

	modeled := stringParts
	if len(stringParts) > pongModeledFieldCount {
		pong.Extra = append([]string(nil), stringParts[pongModeledFieldCount:]...)
		modeled = stringParts[:pongModeledFieldCount]
	}

	pongParts := make([]interface{}, len(modeled))
	for i, val := range modeled {
		pongParts[i] = val
	}

	util.MapFieldsToStruct(pongParts, &pong)

	return pong
}

// Turns a PongData into a string that complies with the Bedrock protocol,
// separating the fields with ;
func writePong(pong PongData) string {
	var pongDataFields []string
	pongDataFieldsRaw := util.MapStructToFields(&pong)
	for i := 0; i < pongModeledFieldCount && i < len(pongDataFieldsRaw); i++ {
		pongDataFields = append(pongDataFields, fmt.Sprintf("%v", pongDataFieldsRaw[i]))
	}
	pongDataFields = append(pongDataFields, pong.Extra...)

	// Ensure that there aren't a bunch of ; on the end, but at least one
	joined := strings.Join(pongDataFields, ";")
	joined = dupeSemicolonRegex.ReplaceAllString(joined, "")
	return fmt.Sprintf("%s;", joined)
}
