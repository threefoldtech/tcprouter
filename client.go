package tcprouter

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync"
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
		c.RemoteConn.Close()
	}

	if c.LocalConn != nil {
		c.LocalConn.Close()
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
		MagicNr: MagicNr,
		Secret:  []byte(c.secret),
	}
	// at this point if the server refuse the hanshake it will
	// just close the connection which should return an error
	return h.Write(c.RemoteConn)
}

func (c *client) Forward() error {

	wg := sync.WaitGroup{}
	wg.Add(2)

	go forward(c.LocalConn, c.RemoteConn, &wg)
	go forward(c.RemoteConn, c.LocalConn, &wg)

	defer func() {
		log.Println("close connections")
		c.RemoteConn.Close()
		c.LocalConn.Close()
	}()

	wg.Wait()
	return nil
}

func forward(dst, src net.Conn, wg *sync.WaitGroup) {
	defer wg.Done()
	io.Copy(dst, src)

	dst.Close()
	src.Close()

	log.Printf("end of copy from %s to %s\n", src.RemoteAddr(), dst.RemoteAddr())
}
