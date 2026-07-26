// Package corpus loads the cross-version compatibility test fixtures.
//
// It contains NO phantom protocol logic - only fixture data and the XFAIL
// registry - which is why both test suites may import it without the black-box
// tier learning anything about phantom's implementation. See test/compat/DESIGN.md.
package corpus

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	_ "embed"
)

//go:embed data/captured.json
var capturedJSON []byte

//go:embed data/synthetic.json
var syntheticJSON []byte

//go:embed data/known_failures.json
var knownFailuresJSON []byte

// Magic is the 16-byte RakNet offline-message magic constant.
var Magic = []byte{
	0x00, 0xff, 0xff, 0x00, 0xfe, 0xfe, 0xfe, 0xfe,
	0xfd, 0xfd, 0xfd, 0xfd, 0x12, 0x34, 0x56, 0x78,
}

// PongID and PingID are the RakNet offline packet IDs phantom cares about.
const (
	PongID     = 0x1c
	PingID     = 0x01
	PingOpenID = 0x02
)

// headerLen is the fixed part of an Unconnected Pong: ID(1) + pingTime(8) +
// serverGUID(8) + magic(16) + strLen(2).
const headerLen = 35

// CapturedEntry is one real pong captured from a bedrock-protocol server.
type CapturedEntry struct {
	MC             string   `json:"mc"`
	Protocol       int      `json:"protocol"`
	PongHex        string   `json:"pongHex"`
	EchoedPingTime string   `json:"echoedPingTime"`
	ServerGUID     string   `json:"serverGUID"`
	MOTD           string   `json:"motd"`
	Fields         []string `json:"fields"`
}

// Frame returns the raw pong datagram bytes.
func (e CapturedEntry) Frame() []byte {
	b, err := hex.DecodeString(e.PongHex)
	if err != nil {
		panic(fmt.Sprintf("corpus: entry %s has invalid pongHex: %v", e.MC, err))
	}
	return b
}

// FieldCount is the number of real MOTD fields, excluding the empty element
// produced by the conventional trailing semicolon.
func (e CapturedEntry) FieldCount() int {
	return realFieldCount(e.MOTD)
}

type capturedDoc struct {
	GeneratedBy string          `json:"generatedBy"`
	PingTime    string          `json:"pingTime"`
	ClientGUID  string          `json:"clientGUID"`
	Entries     []CapturedEntry `json:"entries"`
}

// SyntheticEntry is a hand-authored fixture: either a MOTD that gets wrapped in
// a well-formed frame, or verbatim (possibly malformed) bytes.
type SyntheticEntry struct {
	ID             string      `json:"id"`
	Desc           string      `json:"desc"`
	MOTD           *string     `json:"motd"`
	MOTDRepeat     *motdRepeat `json:"motdRepeat"`
	RawHex         *string     `json:"rawHex"`
	FieldCount     int         `json:"fieldCount"`
	WantParseError bool        `json:"wantParseError"`
	NoRoundTrip    bool        `json:"noRoundTrip"`
	Tags           []string    `json:"tags"`

	// Frame surgery. These build an otherwise complete, well-formed pong with
	// exactly one thing wrong, so a fixture tests the defect it names rather
	// than tripping an unrelated length check first.
	PacketIDOverride *int    `json:"packetIDOverride"`
	MagicOverride    *string `json:"magicOverride"`

	frame []byte
	motd  string
}

type motdRepeat struct {
	Prefix string `json:"prefix"`
	Unit   string `json:"unit"`
	Count  int    `json:"count"`
	Suffix string `json:"suffix"`
}

// Frame returns the datagram bytes for this fixture.
func (e SyntheticEntry) Frame() []byte { return e.frame }

// IsRaw reports whether this fixture supplies verbatim bytes rather than a
// generated well-formed frame.
func (e SyntheticEntry) IsRaw() bool { return e.RawHex != nil }

// MOTDString returns the MOTD this fixture carries. Meaningless for raw fixtures.
func (e SyntheticEntry) MOTDString() string { return e.motd }

// HasTag reports whether the fixture carries the given tag.
func (e SyntheticEntry) HasTag(tag string) bool {
	for _, t := range e.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

type syntheticDoc struct {
	PingTime   string           `json:"pingTime"`
	ServerGUID string           `json:"serverGUID"`
	Entries    []SyntheticEntry `json:"entries"`
}

// KnownFailure is one registered XFAIL.
type KnownFailure struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
	TODO   string `json:"todo"`
}

type knownFailuresDoc struct {
	Failures []KnownFailure `json:"failures"`
}

var (
	captured  capturedDoc
	synthetic syntheticDoc
	known     knownFailuresDoc
)

func init() {
	mustUnmarshal(capturedJSON, &captured, "captured.json")
	mustUnmarshal(syntheticJSON, &synthetic, "synthetic.json")
	mustUnmarshal(knownFailuresJSON, &known, "known_failures.json")

	pingTime := mustHex(synthetic.PingTime, "synthetic pingTime")
	guid := mustHex(synthetic.ServerGUID, "synthetic serverGUID")

	for i := range synthetic.Entries {
		e := &synthetic.Entries[i]
		switch {
		case e.RawHex != nil:
			e.frame = mustHex(*e.RawHex, "entry "+e.ID)
		case e.MOTDRepeat != nil:
			r := e.MOTDRepeat
			e.motd = r.Prefix + strings.Repeat(r.Unit, r.Count) + r.Suffix
			e.frame = e.build(pingTime, guid)
		case e.MOTD != nil:
			e.motd = *e.MOTD
			e.frame = e.build(pingTime, guid)
		default:
			panic(fmt.Sprintf("corpus: synthetic entry %q has none of motd/motdRepeat/rawHex", e.ID))
		}
	}
}

func mustUnmarshal(data []byte, v interface{}, name string) {
	if err := json.Unmarshal(data, v); err != nil {
		panic(fmt.Sprintf("corpus: parsing %s: %v", name, err))
	}
}

func mustHex(s, what string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(fmt.Sprintf("corpus: %s is not valid hex: %v", what, err))
	}
	return b
}

// build assembles this fixture's frame, applying any single-field surgery.
func (e SyntheticEntry) build(pingTime, serverGUID []byte) []byte {
	id := byte(PongID)
	if e.PacketIDOverride != nil {
		id = byte(*e.PacketIDOverride)
	}
	magic := Magic
	if e.MagicOverride != nil {
		magic = mustHex(*e.MagicOverride, "magicOverride for "+e.ID)
		if len(magic) != len(Magic) {
			panic(fmt.Sprintf("corpus: %s magicOverride must be %d bytes, got %d",
				e.ID, len(Magic), len(magic)))
		}
	}
	return buildPong(id, pingTime, serverGUID, magic, e.motd)
}

// BuildPong assembles a well-formed Unconnected Pong datagram around a MOTD.
func BuildPong(pingTime, serverGUID []byte, motd string) []byte {
	return buildPong(PongID, pingTime, serverGUID, Magic, motd)
}

func buildPong(id byte, pingTime, serverGUID, magic []byte, motd string) []byte {
	out := make([]byte, 0, headerLen+len(motd))
	out = append(out, id)
	out = append(out, pingTime...)
	out = append(out, serverGUID...)
	out = append(out, magic...)

	var lenBuf [2]byte
	binary.BigEndian.PutUint16(lenBuf[:], uint16(len(motd)))
	out = append(out, lenBuf[:]...)
	return append(out, motd...)
}

// SplitMOTD extracts the MOTD string from a well-formed pong datagram.
func SplitMOTD(frame []byte) (string, error) {
	if len(frame) < headerLen {
		return "", fmt.Errorf("frame too short: %d bytes", len(frame))
	}
	n := int(binary.BigEndian.Uint16(frame[33:35]))
	if len(frame) < headerLen+n {
		return "", fmt.Errorf("declared MOTD length %d exceeds %d remaining bytes", n, len(frame)-headerLen)
	}
	return string(frame[headerLen : headerLen+n]), nil
}

// realFieldCount counts MOTD fields, ignoring the empty element produced by a
// conventional trailing semicolon.
func realFieldCount(motd string) int {
	parts := strings.Split(motd, ";")
	if n := len(parts); n > 0 && parts[n-1] == "" {
		return n - 1
	}
	return len(parts)
}

// RealFieldCount is the exported form of realFieldCount.
func RealFieldCount(motd string) int { return realFieldCount(motd) }

// Captured returns every per-version captured pong, ascending by protocol number.
func Captured() []CapturedEntry { return captured.Entries }

// CapturedPingTime is the ping time the generator sent; servers must echo it.
func CapturedPingTime() []byte { return mustHex(captured.PingTime, "captured pingTime") }

// GeneratedBy identifies the tool and version that produced the captured corpus.
func GeneratedBy() string { return captured.GeneratedBy }

// Synthetic returns every hand-authored fixture.
func Synthetic() []SyntheticEntry { return synthetic.Entries }

// SyntheticPingTime and SyntheticGUID are the values baked into generated frames.
func SyntheticPingTime() []byte { return mustHex(synthetic.PingTime, "synthetic pingTime") }
func SyntheticGUID() []byte     { return mustHex(synthetic.ServerGUID, "synthetic serverGUID") }

// KnownFailures returns the whole XFAIL registry.
func KnownFailures() []KnownFailure { return known.Failures }

// LookupKnownFailure returns the registered failure for an id, if any.
func LookupKnownFailure(id string) (KnownFailure, bool) {
	for _, f := range known.Failures {
		if f.ID == id {
			return f, true
		}
	}
	return KnownFailure{}, false
}
