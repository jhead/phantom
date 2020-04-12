package proto

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

var ConnectionRequestTwoID byte = 0x07

type ConnectionRequestTwo struct {
	Magic     []byte
	IPVersion uint8
	Address   []byte
	Port      uint16
	MTU       uint16
	ClientID  []byte
}

func ReadConnectionRequestTwo(in []byte) (res *ConnectionRequestTwo, err error) {
	if len(in) < 34 {
		return nil, fmt.Errorf("ConnectionRequestTwo packet size invalid: %d", len(in))
	}

	res = &ConnectionRequestTwo{}
	buf := bytes.NewBuffer(in)

	// Packet ID
	buf.ReadByte()

	// Magic
	if res.Magic, err = readMagic(buf); err != nil {
		return nil, err
	}

	// IP version
	if res.IPVersion, err = buf.ReadByte(); err != nil {
		return nil, err
	}

	ipLen := 4
	if res.IPVersion == 6 {
		ipLen = 16
	}

	res.Address = make([]byte, ipLen)
	if _, err := buf.Read(res.Address); err != nil {
		return nil, err
	}

	// Port
	portBytes := make([]byte, 2)
	if _, err := buf.Read(portBytes); err != nil {
		return nil, err
	}

	res.Port = binary.BigEndian.Uint16(portBytes)

	// MTU
	mtuBytes := make([]byte, 2)
	if _, err := buf.Read(mtuBytes); err != nil {
		return nil, err
	}

	res.MTU = binary.BigEndian.Uint16(mtuBytes)

	// Client ID
	res.ClientID = make([]byte, 8)
	if _, err := buf.Read(res.ClientID); err != nil {
		return nil, err
	}

	return
}

func (req ConnectionRequestTwo) Build() bytes.Buffer {
	var outBuffer bytes.Buffer

	outBuffer.WriteByte(ConnectionRequestTwoID)
	outBuffer.Write(req.Magic)
	outBuffer.WriteByte(req.IPVersion)

	fmt.Println(req.Address)
	outBuffer.Write(req.Address)

	// Port
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, req.Port)
	outBuffer.Write(portBytes)

	// MTU
	mtuBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(mtuBytes, req.MTU)
	outBuffer.Write(mtuBytes)

	// Client ID
	outBuffer.Write(req.ClientID)

	return outBuffer
}
