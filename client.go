package tcprouter

import (
	"fmt"
	"io"
	"net"

	"github.com/rs/zerolog/log"
)

type Client struct {
	// connection to the tcp router server
	RemoteConn net.Conn
	// connection to the local application
	LocalConn net.Conn

	// secret used to identify the connection in the tcp router server
	secret []byte
}

// NewClient creates a new TCP router client
func NewClient(secret string) *Client {
	return &Client{
		secret: []byte(secret),
	}
}

func (c *Client) ConnectRemote(addr string) error {
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

func (c *Client) ConnectLocal(addr string) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}

	c.LocalConn = conn

	return nil
}

func (c *Client) Handshake() error {
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

func (c *Client) Forward() {

	cErr := make(chan error)
	defer func() {
		c.RemoteConn.Close()
		c.LocalConn.Close()
	}()

	go forward(c.LocalConn, c.RemoteConn, cErr)
	go forward(c.RemoteConn, c.LocalConn, cErr)

	err := <-cErr
	if err != nil {
		log.Error().Err(err).Msg("Error during connection")
	}

	<-cErr
}

func forward(dst, src net.Conn, cErr chan<- error) {
	_, err := io.Copy(dst, src)
	cErr <- err

	tcpConn, ok := dst.(*net.TCPConn)
	if ok {
		if err := tcpConn.CloseWrite(); err != nil {
			log.Error().Err(err).Msg("Error while terminating connection")
		}
	}
}
