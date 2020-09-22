package proto

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"regexp"
	"strings"

	"github.com/jhead/phantom/membrane/internal/util"
)

var UnconnectedPingID byte = 0x01
var UnconnectedPongID byte = 0x1C

type UnconnectedPing struct {
	PingTime []byte
	ID       []byte
	Magic    []byte
	Pong     PongData
}

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
}

var OfflinePong = UnconnectedPing{
	PingTime: []byte{0, 0, 0, 0, 0, 0, 0, 0},
	ID:       []byte{0, 0, 0, 0, 0, 0, 0, 0},
	Magic:    []byte{0x00, 0xff, 0xff, 0x00, 0xfe, 0xfe, 0xfe, 0xfe, 0xfd, 0xfd, 0xfd, 0xfd, 0x12, 0x34, 0x56, 0x78},
	Pong: PongData{
		Edition:         "MCPE",
		MOTD:            "phantom §cServer offline",
		ProtocolVersion: "390",
		Version:         "1.14.60",
		Players:         "0",
		MaxPlayers:      "0",
		GameType:        "Creative",
		NintendoLimited: "1",
	},
}.Build()

var dupeSemicolonRegex = regexp.MustCompile(";{2,}$")

func ReadUnconnectedPing(in []byte) (reply *UnconnectedPing, err error) {
	reply = &UnconnectedPing{}
	buf := bytes.NewBuffer(in)

	// Packet ID
	buf.ReadByte()

	reply.PingTime = make([]byte, 8)
	if _, err := buf.Read(reply.PingTime); err != nil {
		return nil, err
	}

	reply.ID = make([]byte, 8)
	if _, err := buf.Read(reply.ID); err != nil {
		return nil, err
	}

	reply.Magic = make([]byte, 16)
	if _, err := buf.Read(reply.Magic); err != nil {
		return nil, err
	}

	pongLenBytes := make([]byte, 2)
	if _, err := buf.Read(pongLenBytes); err != nil {
		return nil, err
	}

	pongLen := binary.BigEndian.Uint16(pongLenBytes)

	pongDataBytes := make([]byte, pongLen)
	if _, err := buf.Read(pongDataBytes); err != nil {
		return nil, err
	}

	reply.Pong = readPong(string(pongDataBytes))

	return
}

func (r UnconnectedPing) Build() bytes.Buffer {
	var outBuffer bytes.Buffer

	outBuffer.WriteByte(UnconnectedPongID)
	outBuffer.Write(r.PingTime)
	outBuffer.Write(r.ID)
	outBuffer.Write(r.Magic)

	pongDataString := writePong(r.Pong)
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
	pongParts := []interface{}{}

	stringParts := strings.Split(raw, ";")
	for _, val := range stringParts {
		pongParts = append(pongParts, val)
	}

	util.MapFieldsToStruct(pongParts, &pong)

	return pong
}

// Turns a PongData into a string that complies with the Bedrock protocol,
// separating the fields with ;
func writePong(pong PongData) string {
	var pongDataFields []string
	pongDataFieldsRaw := util.MapStructToFields(&pong)
	for _, value := range pongDataFieldsRaw {
		stringValue := fmt.Sprintf("%v", value)
		pongDataFields = append(pongDataFields, stringValue)
	}

	// Ensure that there aren't a bunch of ; on the end, but at least one
	joined := strings.Join(pongDataFields, ";")
	joined = dupeSemicolonRegex.ReplaceAllString(joined, "")
	return fmt.Sprintf("%s;", joined)
}
