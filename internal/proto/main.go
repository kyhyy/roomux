package proto

import (
	"encoding/binary"
	"errors"
	"io"
)

// Attach, Who and History are opening frames: the first frame on a connection
// tells the daemon what the connection is for. Input, Resize, NewWindow,
// SelectWindow and CycleWindow can come at any time after an Attach frame.
const (
	Input        byte = 0
	Resize       byte = 1
	Attach       byte = 2
	Who          byte = 3
	History      byte = 4
	NewWindow    byte = 5
	SelectWindow byte = 6
	CycleWindow  byte = 7
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

// IndexPayload makes the payload of a select-window frame.
func IndexPayload(index uint16) []byte {
	payload := make([]byte, 2)
	binary.BigEndian.PutUint16(payload, index)
	return payload
}

// ParseIndex gets the window index from a select-window payload.
func ParseIndex(payload []byte) (uint16, error) {
	if len(payload) != 2 {
		return 0, errors.New("proto: bad index payload")
	}
	return binary.BigEndian.Uint16(payload), nil
}

// DeltaPayload makes the payload of a cycle-window frame. A positive delta
// moves to a later window, and a negative delta moves to an earlier window.
func DeltaPayload(delta int8) []byte {
	return []byte{byte(delta)}
}

// ParseDelta gets the delta from a cycle-window payload.
func ParseDelta(payload []byte) (int8, error) {
	if len(payload) != 1 {
		return 0, errors.New("proto: bad delta payload")
	}
	return int8(payload[0]), nil
}
