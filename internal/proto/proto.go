package proto

import "bytes"

func readMagic(buf *bytes.Buffer) ([]byte, error) {
	in := make([]byte, 16)

	if _, err := buf.Read(in); err != nil {
		return nil, err
	}

	return in, nil
}
