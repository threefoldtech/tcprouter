package main

import (
	"fmt"
	"io"
	"log"
	"net"
)

type client struct {
	// connection to the tcp router server
	RemoteConn net.Conn
	// connection to the local application
	LocalConn net.Conn

	// secret used to identify the connection in the tcp router server
	secret []byte
}

// NewClient creates a new TCP router client
func NewClient(secret string) *client {
	return &client{
		secret: []byte(secret),
	}
}

func (c *client) Close() {
	if c.RemoteConn != nil {
		if err := c.RemoteConn.Close(); err != nil {
			log.Printf("error closing remote connection to %s", c.RemoteConn.RemoteAddr().String())
		}
	}

	if c.LocalConn != nil {
		if err := c.LocalConn.Close(); err != nil {
			log.Printf("error closing local connection to %s", c.LocalConn.RemoteAddr().String())
		}
	}
}

func (c *client) ConnectRemote(addr string) error {
	if len(c.secret) == 0 {
		return fmt.Errorf("no secret configured")
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}

	c.RemoteConn = conn

	return nil
}

func (c *client) ConnectLocal(addr string) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}

	c.LocalConn = conn

	return nil
}

func (c *client) Handshake() error {
	if c.RemoteConn == nil {
		return fmt.Errorf("not connected")
	}

	h := Handshake{
		MagicNr: magicNr,
		Secret:  [256]byte{},
	}
	copy(h.Secret[:], c.secret)
	// at this point if the server refuse the hanshake it will
	// just close the connection which should return an error
	return h.Write(c.RemoteConn)
}

func (c *client) Forward() error {

	cErr := make(chan error)
	go forward(c.LocalConn, c.RemoteConn, cErr)
	go forward(c.RemoteConn, c.LocalConn, cErr)

	defer func() {
		c.RemoteConn.Close()
		c.LocalConn.Close()
		close(cErr)
	}()

	err := <-cErr
	if err != nil {
		fmt.Printf("Error during connection: %v", err)
		return err
	}
	return nil
}

func forward(dst io.Writer, src io.Reader, cErr chan<- error) {
	_, err := io.Copy(dst, src)
	cErr <- err
}
