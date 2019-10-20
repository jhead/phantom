package proto

import (
	"bytes"
	"encoding/binary"
)

var UnconnectedReplyID byte = 0x1C

type UnconnectedReply struct {
	PingTime   []byte
	ID         []byte
	Magic      []byte
	ServerName string
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

	serverNameLenBytes := make([]byte, 2)
	if _, err := buf.Read(serverNameLenBytes); err != nil {
		return nil, err
	}

	serverNameLen := binary.BigEndian.Uint16(serverNameLenBytes)

	serverNameBytes := make([]byte, serverNameLen)
	if _, err := buf.Read(serverNameBytes); err != nil {
		return nil, err
	}

	reply.ServerName = string(serverNameBytes)

	return
}

func (r UnconnectedReply) Build() bytes.Buffer {
	var outBuffer bytes.Buffer

	outBuffer.WriteByte(UnconnectedReplyID)
	outBuffer.Write(r.PingTime)
	outBuffer.Write(r.ID)
	outBuffer.Write(r.Magic)

	serverNameLen := uint16(len(r.ServerName))
	stringBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(stringBuf, serverNameLen)

	outBuffer.Write(stringBuf)
	outBuffer.WriteString(r.ServerName)

	return outBuffer
}
