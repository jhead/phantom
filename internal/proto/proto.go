package proto

import (
	"bytes"
	"encoding/binary"
)

var UnconnectedRequestID byte = 0x01
var UnconnectedReplyID byte = 0x1C

type UnconnectedReply struct {
	ID         []byte
	Magic      []byte
	ServerName string
}

func (r UnconnectedReply) Build() bytes.Buffer {
	var outBuffer bytes.Buffer

	outBuffer.WriteByte(UnconnectedReplyID)
	outBuffer.Write(r.ID)
	outBuffer.Write(r.ID)
	outBuffer.Write(r.Magic)

	serverNameLen := uint16(len(r.ServerName))
	stringBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(stringBuf, serverNameLen)

	outBuffer.Write(stringBuf)
	outBuffer.WriteString(r.ServerName)

	return outBuffer
}
