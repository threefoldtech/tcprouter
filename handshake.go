package tcprouter

import (
	"encoding/binary"
	"io"
)

const (
	// TODO: chose a valid magic number
	magicNr = 0x1111
)

// Handshake is the struct used to serialize the first frame sent to the server
type Handshake struct {
	MagicNr uint16
	Secret  [256]byte
}

func (h Handshake) Write(w io.Writer) error {
	return binary.Write(w, binary.BigEndian, h)
}

func (h *Handshake) Read(r io.Reader) error {
	if h == nil {
		h = &Handshake{}
	}
	return binary.Read(r, binary.BigEndian, h)
}
