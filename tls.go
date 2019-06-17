package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net"
)

// Code extracted from traefik to get the servername from TLS connection.
// GetConn creates a connection proxy with a peeked string
func GetConn(conn net.Conn, peeked string) net.Conn {
	conn = &Conn{
		Peeked: []byte(peeked),
		Conn:   conn,
	}
	return conn
}

// Conn is a connection proxy that handles Peeked bytes
type Conn struct {
	// Peeked are the bytes that have been read from Conn for the
	// purposes of route matching, but have not yet been consumed
	// by Read calls. It set to nil by Read when fully consumed.
	Peeked []byte

	// Conn is the underlying connection.
	// It can be type asserted against *net.TCPConn or other types
	// as needed. It should not be read from directly unless
	// Peeked is nil.
	net.Conn
}

// Read reads bytes from the connection (using the buffer prior to actually reading)
func (c *Conn) Read(p []byte) (n int, err error) {
	if len(c.Peeked) > 0 {
		n = copy(p, c.Peeked)
		c.Peeked = c.Peeked[n:]
		if len(c.Peeked) == 0 {
			c.Peeked = nil
		}
		return n, nil
	}
	return c.Conn.Read(p)
}

// clientHelloServerName returns the SNI server name inside the TLS ClientHello,
// without consuming any bytes from br.
// On any error, the empty string is returned.
func clientHelloServerName(br *bufio.Reader) (string, bool, string) {
	hdr, err := br.Peek(1)
	if err != nil {
		if err != io.EOF {
			fmt.Println("Error while Peeking first byte: %s", err)
		}
		return "", false, ""
	}
	const recordTypeHandshake = 0x16
	if hdr[0] != recordTypeHandshake {
		// log.Errorf("Error not tls")
		return "", false, getPeeked(br) // Not TLS.
	}

	const recordHeaderLen = 5
	hdr, err = br.Peek(recordHeaderLen)
	if err != nil {
		fmt.Println("Error while Peeking hello: %s", err)
		return "", false, getPeeked(br)
	}
	recLen := int(hdr[3])<<8 | int(hdr[4]) // ignoring version in hdr[1:3]
	helloBytes, err := br.Peek(recordHeaderLen + recLen)
	if err != nil {
		fmt.Println("Error while Hello: %s", err)
		return "", true, getPeeked(br)
	}
	sni := ""
	server := tls.Server(sniSniffConn{r: bytes.NewReader(helloBytes)}, &tls.Config{
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			sni = hello.ServerName
			return nil, nil
		},
	})
	_ = server.Handshake()
	return sni, true, getPeeked(br)
}

func getPeeked(br *bufio.Reader) string {
	peeked, err := br.Peek(br.Buffered())
	if err != nil {
		fmt.Println("Could not get anything: %s", err)
		return ""
	}
	return string(peeked)
}

// sniSniffConn is a net.Conn that reads from r, fails on Writes,
// and crashes otherwise.
type sniSniffConn struct {
	r        io.Reader
	net.Conn // nil; crash on any unexpected use
}

// Read reads from the underlying reader
func (c sniSniffConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// Write crashes all the time
func (sniSniffConn) Write(p []byte) (int, error) { return 0, io.EOF }

func connCopy(dst, src net.Conn, errCh chan error) {
	_, err := io.Copy(dst, src)
	errCh <- err
}
