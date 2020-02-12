package tcprouter

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/libp2p/go-yamux"
	"github.com/rs/zerolog/log"
)

// Client connect to a tpc router server and opens a reverse tunnel
type Client struct {
	localAddr  string
	remoteAddr string
	// secret used to identify the connection in the tcp router server
	secret []byte

	// connection to the tcp router server
	remoteSession *yamux.Session
}

// NewClient creates a new TCP router client
func NewClient(secret, local, remote string) *Client {
	return &Client{
		localAddr:  local,
		remoteAddr: remote,
		secret:     []byte(secret),
	}
}

// Start starts the client by opening a connection to the router server, doing the handshake
// then start listening for incoming steam from the router server
func (c Client) Start(ctx context.Context) error {
	if err := c.connectRemote(c.remoteAddr); err != nil {
		return fmt.Errorf("failed to connect to TCP router server: %w", err)
	}

	log.Info().Msg("start handshake")
	if err := c.handshake(); err != nil {
		return fmt.Errorf("failed to handshake with TCP router server: %w", err)
	}
	log.Info().Msg("handshake done")

	return c.listen(ctx)
}

func (c *Client) connectRemote(addr string) error {
	if len(c.secret) == 0 {
		return fmt.Errorf("no secret configured")
	}

	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return err
	}

	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return err
	}

	// Setup client side of yamux
	session, err := yamux.Client(conn, nil)
	if err != nil {
		panic(err)
	}

	c.remoteSession = session

	return nil
}

func (c *Client) connectLocal(addr string) (WriteCloser, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (c *Client) handshake() error {
	if c.remoteSession == nil {
		return fmt.Errorf("not connected")
	}

	h := Handshake{
		MagicNr: MagicNr,
		Secret:  []byte(c.secret),
	}
	// at this point if the server refuse the handshake it will
	// just close the connection which should return an error
	stream, err := c.remoteSession.OpenStream()
	if err != nil {
		return err
	}
	defer stream.Close()

	return h.Write(stream)
}

func (c *Client) listen(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cCon := make(chan WriteCloser)
	cErr := make(chan error)
	go func(ctx context.Context, cCon chan<- WriteCloser, cErr chan<- error) {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				conn, err := c.remoteSession.AcceptStream()
				if err != nil {
					cErr <- err
					return
				}
				cCon <- WrapConn(conn)
			}
		}
	}(ctx, cCon, cErr)

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-cErr:
			return fmt.Errorf("accept connection failed: %w", err)
		case remote := <-cCon:
			log.Info().
				Str("remote add", remote.RemoteAddr().String()).
				Msg("incoming stream, connect to local application")

			local, err := c.connectLocal(c.localAddr)
			if err != nil {
				return fmt.Errorf("failed to connect to local application: %w", err)
			}

			go func(remote, local WriteCloser) {
				log.Info().Msg("start forwarding")

				cErr := make(chan error)
				go forward(local, remote, cErr)
				go forward(remote, local, cErr)

				err = <-cErr
				if err != nil {
					log.Error().Err(err).Msg("Error during forwarding: %w")
				}

				<-cErr

				if err := remote.Close(); err != nil {
					log.Error().Err(err).Msg("Error while terminating connection")
				}
				if err := local.Close(); err != nil {
					log.Error().Err(err).Msg("Error while terminating connection")
				}
			}(remote, local)
		}
	}
}

func forward(dst, src WriteCloser, cErr chan<- error) {
	_, err := io.Copy(dst, src)
	cErr <- err
	if err := dst.CloseWrite(); err != nil {
		log.Error().Err(err).Msgf("error closing %s", dst.RemoteAddr().String())
	}
}

type wrappedCon struct {
	*yamux.Stream
}

// WrapConn wraps a stream into a wrappedCon so it implements the WriteCloser interface
func WrapConn(conn *yamux.Stream) WriteCloser {
	return wrappedCon{conn}
}

func (c wrappedCon) CloseWrite() error {
	return c.Stream.Close()
}
