package tcprouter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-yamux"
	"github.com/rs/zerolog/log"

	"github.com/abronan/valkeyrie/store"
)

// ServerOptions hold the configuration of server listeners
type ServerOptions struct {
	ListeningAddr           string
	ListeningTLSPort        uint
	ListeningHTTPPort       uint
	ListeningForClientsPort uint
}

// HTTPAddr returns the HTTP listener address
func (o ServerOptions) HTTPAddr() string {
	return fmt.Sprintf("%s:%d", o.ListeningAddr, o.ListeningHTTPPort)
}

// TLSAddr returns the TLS listener address
func (o ServerOptions) TLSAddr() string {
	return fmt.Sprintf("%s:%d", o.ListeningAddr, o.ListeningTLSPort)
}

// ClientsAddr returns the client listener address
func (o ServerOptions) ClientsAddr() string {
	return fmt.Sprintf("%s:%d", o.ListeningAddr, o.ListeningForClientsPort)
}

//Server is tcp router server
type Server struct {
	ServerOptions ServerOptions
	DbStore       store.Store
	Services      map[string]Service

	activeConnections map[string]*yamux.Session

	listeners   []net.Listener
	listenersMU sync.Mutex
	wg          sync.WaitGroup
}

// NewServer creates a new server
func NewServer(forwardOptions ServerOptions, store store.Store, services map[string]Service) *Server {
	if services == nil {
		services = make(map[string]Service)
	}

	return &Server{
		ServerOptions:     forwardOptions,
		Services:          services,
		DbStore:           store,
		activeConnections: make(map[string]*yamux.Session),
		listeners:         []net.Listener{},
	}
}

// Start starts the server and blocks forever
func (s *Server) Start(ctx context.Context) error {

	s.wg.Add(3)
	go s.listen(ctx, s.ServerOptions.HTTPAddr(), HandlerFunc(s.handleHTTPConnection))
	go s.listen(ctx, s.ServerOptions.TLSAddr(), HandlerFunc(s.handleConnection))
	go s.listen(ctx, s.ServerOptions.ClientsAddr(), HandlerFunc(s.handleTCPRouterClientConnection))

	s.wg.Wait()
	log.Info().Msg("stopping server...")
	s.listenersMU.Lock()
	defer s.listenersMU.Unlock()
	for _, ln := range s.listeners {
		if ln != nil {
			if err := ln.Close(); err != nil {
				log.Error().Err(err).Msg("error closing connection")
			}
		}
	}
	log.Info().Msg("stopped")

	return nil
}

func (s *Server) listen(ctx context.Context, addr string, handler Handler) {
	defer s.wg.Done()

	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		log.Fatal().
			Err(err).
			Str("addr", addr).
			Msg("failed to resolve addr")
	}

	ln, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		log.Fatal().
			Err(err).
			Str("addr", addr).
			Msg("failed to start listener")
	}

	s.listenersMU.Lock()
	s.listeners = append(s.listeners, tcpKeepAliveListener{ln})
	s.listenersMU.Unlock()

	for {
		select {
		case <-ctx.Done():
			return

		default:
			ln.SetDeadline(time.Now().Add(time.Second))
			conn, err := ln.AcceptTCP()
			if err != nil {
				if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
					continue
				}
				log.Fatal().Err(err).Msg("Failed to accept connection")
			}

			go handler.ServeTCP(conn)
		}
	}
}

func (s *Server) getHost(host string) (Service, error) {
	service := Service{}

	key := fmt.Sprintf("tcprouter/service/%s", host)
	servicePair, err := s.DbStore.Get(key, nil)
	if err != nil {
		return service, fmt.Errorf("host not found at key %s: %v", key, err)
	}

	err = json.Unmarshal(servicePair.Value, &service)
	if err != nil {
		return service, fmt.Errorf("invalid service content")
	}

	log.Debug().
		Str("key", key).
		Str("service", fmt.Sprintf("%v", service)).
		Msg("service found")
	return service, nil
}

func (s *Server) handleTCPRouterClientConnection(conn WriteCloser) {
	session, err := yamux.Server(conn, nil)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}

	stream, err := session.Accept()
	if err != nil {
		log.Error().Err(err).Send()
		return
	}
	defer stream.Close()

	hs := &Handshake{}
	if err := hs.Read(stream); err != nil {
		log.Error().Err(err).Msg("handshake failed")
		conn.Close()
		return
	}
	if hs.MagicNr != MagicNr {
		log.Error().Msgf("expected %d MagicNr and received %d", MagicNr, hs.MagicNr)
		conn.Close()
		return
	}
	log.Info().
		Str("remote addr", conn.RemoteAddr().String()).
		Msg("handshake done... adding to active connections")

	s.activeConnections[string(hs.Secret[:])] = session
}

func (s *Server) handleConnection(conn WriteCloser) {
	br := bufio.NewReader(conn)
	serverName, isTLS, peeked := clientHelloServerName(br)
	log.Info().
		Str("server name", serverName).
		Bool("is TLS", isTLS).
		Msg("connection analyzed")

	if err := s.handleService(conn, serverName, peeked, isTLS); err != nil {
		log.Error().
			Str("server name", serverName).
			Err(err).
			Msg("error forwarding traffic")
	}
}

func (s *Server) handleHTTPConnection(conn WriteCloser) {
	br := bufio.NewReader(conn)
	peeked := ""
	host := ""
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			log.Error().Err(err).Msg("failed to decode HTTP header")
			return
		}
		peeked = peeked + line
		if strings.HasPrefix(line, "Host:") {
			host = strings.Trim(line[6:], " \n\r")
			if strings.Contains(host, ":") {
				host, _, err = net.SplitHostPort(host)
				if err != nil {
					log.Error().
						Err(err).
						Str("server name", host).
						Msg("failed to parse split host port from server name")
					return
				}
			}
		}
		if strings.Trim(line, " \n\r") == "" {
			break
		}

	}
	if host == "" {
		log.Error().Msg("could not find host in HTTP header")
		conn.Close()
		return
	}
	peeked += getPeeked(br)
	log.Info().Msgf("Host found: '%s'", host)
	if err := s.handleService(conn, host, peeked, false); err != nil {
		log.Error().
			Str("server name", host).
			Err(err).
			Msg("error forwarding traffic")
	}
}

func (s *Server) handleService(incoming WriteCloser, serverName, peeked string, isTLS bool) error {
	serverName = strings.ToLower(serverName)
	service, exists := s.Services[serverName]
	if !exists {
		log.Info().Msg("not found in file config, try to load it from db backend")
		var err error
		service, err = s.getHost(serverName)
		exists = err == nil
	}

	if !exists {
		service, exists = s.Services["CATCH_ALL"]
		if !exists {
			incoming.Close()
			return fmt.Errorf("service doesn't exist: %v and no 'CATCH_ALL' service for request", service)
		}
	}

	log.Info().Str("service", fmt.Sprintf("%v", service)).Msg("service found")

	incoming = GetConn(incoming, peeked)
	var (
		outgoing WriteCloser
		err      error
	)

	if service.ClientSecret != "" {
		// retrive an active connection and forward traffic on it
		activeConn, ok := s.activeConnections[service.ClientSecret]
		if !ok {
			incoming.Close()
			return fmt.Errorf("no active connection for service %s", serverName)
		}

		log.Info().Msgf("open new stream to client %s", serverName)
		stream, err := activeConn.OpenStream()
		if err != nil {
			incoming.Close()
			return fmt.Errorf("failed to open stream: %w", err)
		}
		outgoing = WrapConn(stream)

	} else {
		// Dial target server and forward traffic on it
		remotePort := service.HTTPPort
		if isTLS {
			remotePort = service.TLSPort
		}
		outgoing, err = net.DialTCP("tcp", nil, &net.TCPAddr{IP: net.ParseIP(service.Addr), Port: remotePort})
		if err != nil {
			incoming.Close()
			return fmt.Errorf("error while connection to service: %v", err)
		}
	}

	forwardConnection(incoming, outgoing)
	return nil
}

func forwardConnection(local, remote WriteCloser) {
	log.Info().
		Str("remote", remote.RemoteAddr().String()).
		Str("local", local.RemoteAddr().String()).
		Msg("forward active connection")

	cErr := make(chan error)
	defer func() {
		local.Close()
		remote.Close()
	}()

	go forward(local, remote, cErr)
	go forward(remote, local, cErr)

	err := <-cErr
	if err != nil {
		log.Error().
			Str("remote", remote.RemoteAddr().String()).
			Str("local", local.RemoteAddr().String()).
			Err(err).
			Msg("Error during connection")
	}

	<-cErr
}

type tcpKeepAliveListener struct {
	*net.TCPListener
}

func (ln tcpKeepAliveListener) Accept() (net.Conn, error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return nil, err
	}

	if err = tc.SetKeepAlive(true); err != nil {
		return nil, err
	}

	if err = tc.SetKeepAlivePeriod(3 * time.Minute); err != nil {
		return nil, err
	}

	return tc, nil
}
