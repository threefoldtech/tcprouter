package tcprouter

import (
	"bytes"
	"net"
	"sync"
	"testing"

	"github.com/magiconair/properties/assert"
	"github.com/stretchr/testify/require"
)

func TestHandshakeEncodeDecode(t *testing.T) {

	h := Handshake{
		MagicNr: MagicNr,
		Secret:  []byte("hello world"),
	}

	b := bytes.Buffer{}
	err := h.Write(&b)
	require.NoError(t, err)

	h2 := &Handshake{}
	err = h2.Read(&b)
	require.NoError(t, err)

	assert.Equal(t, h.MagicNr, h2.MagicNr)
	assert.Equal(t, h.Secret, h2.Secret)
}

func TestHandshake(t *testing.T) {
	hc := Handshake{
		MagicNr: MagicNr,
		Secret:  []byte("hello world"),
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := l.Accept()
		require.NoError(t, err)

		hs := Handshake{}
		err = hs.Read(conn)
		require.NoError(t, err)

		assert.Equal(t, hs.MagicNr, hc.MagicNr)
		assert.Equal(t, hs.Secret, hc.Secret)
	}()

	conn, err := net.Dial("tcp", l.Addr().String())
	require.NoError(t, err)

	err = hc.Write(conn)
	require.NoError(t, err)

	wg.Wait()
}
