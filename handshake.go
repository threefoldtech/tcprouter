package tcprouter

import (
	"encoding/binary"
	"io"
)

const (
	// TODO: chose a valid magic number
	MagicNr = 0x1111
)

// Handshake is the struct used to serialize the first frame sent to the server
type Handshake struct {
	MagicNr uint16
	Secret  []byte
}

func (h Handshake) Write(w io.Writer) error {
	b := make([]byte, 4+len(h.Secret))
	binary.BigEndian.PutUint16(b[:2], h.MagicNr)
	binary.BigEndian.PutUint16(b[2:4], uint16(len(h.Secret)))
	copy(b[4:], h.Secret)
	_, err := w.Write(b)
	return err
}

func (h *Handshake) Read(r io.Reader) error {
	b := make([]byte, 4)
	n, err := r.Read(b)
	if err != nil {
		return err
	}
	b = b[:n]
	h.MagicNr = binary.BigEndian.Uint16(b[:2])
	size := binary.BigEndian.Uint16(b[2:4])

	b = make([]byte, size)
	n, err = r.Read(b)
	if err != nil {
		return err
	}
	b = b[:n]

	h.Secret = make([]byte, size)
	copy(h.Secret, b[:n])
	return nil
}
