package proto

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/jhead/phantom/internal/util"
)

var UnconnectedReplyID byte = 0x1C

type UnconnectedReply struct {
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
	// Specifically omit these two because they cause issues
	// Port4           string
	// Port6           string
}

func ReadUnconnectedReply(in []byte) (reply *UnconnectedReply, err error) {
	reply = &UnconnectedReply{}
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

func (r UnconnectedReply) Build() bytes.Buffer {
	var outBuffer bytes.Buffer

	outBuffer.WriteByte(UnconnectedReplyID)
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

	fmt.Printf("%v\n", pong)
	return pong
}

// Turns a PongData into a string that complies with the Bedrock protocol,
// separating the fields with ;
func writePong(pong PongData) string {
	var pongDataFields []string
	pongDataFieldsRaw := util.MapStructToFields(&pong)
	for _, value := range pongDataFieldsRaw {
		pongDataFields = append(pongDataFields, fmt.Sprintf("%v", value))
	}

	return strings.Join(pongDataFields, ";")
}
