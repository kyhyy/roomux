package proto

import (
	"encoding/binary"
	"errors"
	"io"
)

// Input and Resize can come at any time. Attach, Who and History are opening
// frames: the first frame tells the daemon what the connection is for.
const (
	Input   byte = 0
	Resize  byte = 1
	Attach  byte = 2
	Who     byte = 3
	History byte = 4
)

// Write sends one frame: the kind, the length of the payload, then the payload.
func Write(w io.Writer, kind byte, payload []byte) error {
	frame := make([]byte, 3+len(payload))
	frame[0] = kind
	binary.BigEndian.PutUint16(frame[1:3], uint16(len(payload)))
	copy(frame[3:], payload)

	_, err := w.Write(frame)
	return err
}

// Read gets the next frame from r.
func Read(r io.Reader) (kind byte, payload []byte, err error) {
	var head [3]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return 0, nil, err
	}

	payload = make([]byte, binary.BigEndian.Uint16(head[1:3]))
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return head[0], payload, nil
}

// ResizePayload makes the payload of a resize frame.
func ResizePayload(rows, cols uint16) []byte {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint16(payload[0:2], rows)
	binary.BigEndian.PutUint16(payload[2:4], cols)
	return payload
}

// ParseResize gets the rows and the columns from a resize payload.
func ParseResize(payload []byte) (rows, cols uint16, err error) {
	if len(payload) != 4 {
		return 0, 0, errors.New("proto: bad resize payload")
	}
	return binary.BigEndian.Uint16(payload[0:2]), binary.BigEndian.Uint16(payload[2:4]), nil
}
