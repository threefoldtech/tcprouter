package tcprouter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"

	"log"

	"github.com/abronan/valkeyrie/store"
)

type ServerOptions struct {
	ListeningAddr           string
	ListeningTLSPort        uint
	ListeningHTTPPort       uint
	ListeningForClientsPort uint
}

func (o ServerOptions) HTTPAddr() string {
	return fmt.Sprintf("%s:%d", o.ListeningAddr, o.ListeningHTTPPort)
}

func (o ServerOptions) TLSAddr() string {
	return fmt.Sprintf("%s:%d", o.ListeningAddr, o.ListeningTLSPort)
}

func (o ServerOptions) ClientsAddr() string {
	return fmt.Sprintf("%s:%d", o.ListeningAddr, o.ListeningForClientsPort)
}

type Server struct {
	ServerOptions     ServerOptions
	DbStore           store.Store
	Services          map[string]Service
	backendM          sync.RWMutex
	activeConnections map[string]net.Conn

	ctx context.Context
	wg  sync.WaitGroup
}

func NewServer(forwardOptions ServerOptions, store store.Store, services map[string]Service) *Server {
	if services == nil {
		services = make(map[string]Service)
	}

	return &Server{
		ServerOptions:     forwardOptions,
		Services:          services,
		DbStore:           store,
		activeConnections: make(map[string]net.Conn),
	}
}

func (s *Server) Start(ctx context.Context) error {
	for key, service := range s.Services {
		s.RegisterService(key, service.Addr, service.ClientSecret, service.TLSPort, service.HTTPPort)
	}

	go s.serveHTTP(ctx)
	go s.serveTLS(ctx)
	go s.serveTCPRouterClients(ctx)

	<-ctx.Done()

	log.Print("stopping server...")
	s.wg.Wait()
	log.Println("stopped")

	return nil
}

func (s *Server) RegisterService(name, remoteAddr, clientSecret string, tlsport int, httpport int) {
	log.Println("register ", name, remoteAddr, clientSecret, tlsport, httpport)
	s.backendM.Lock()
	defer s.backendM.Unlock()
	s.Services[name] = Service{Addr: remoteAddr, ClientSecret: clientSecret, TLSPort: tlsport, HTTPPort: httpport}
}

func (s *Server) DeleteService(name string) {
	s.backendM.Lock()
	defer s.backendM.Unlock()
	delete(s.Services, name)
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
		log.Println("invalid service content.")
		return service, fmt.Errorf("Invalid service content")
	}

	log.Printf("service found at key %s: %v\n", key, service)
	return service, nil
}

func (s *Server) serveHTTP(ctx context.Context) {
	addr := s.ServerOptions.HTTPAddr()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to start tcp listener at  %s: %v", addr, err)
	}

	for {
		select {
		case <-ctx.Done():
			ln.Close()
			return

		default:
			conn, err := ln.Accept()
			if err != nil {
				log.Printf("error accepting TLS connection: %v\n", err)
				continue
			}
			// TODO: need to track these connection and close then when needed
			go s.handleHTTPConnection(conn)
		}
	}
}

func (s *Server) serveTLS(ctx context.Context) {
	addr := s.ServerOptions.TLSAddr()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to start tcp listener at %s: %v ", addr, err)
	}

	log.Println("started TLS server..")
	for {
		select {
		case <-ctx.Done():
			ln.Close()
			return

		default:
			conn, err := ln.Accept()
			if err != nil {
				log.Printf("error accepting TLS connection: %v\n", err)
				continue
			}
			// TODO: need to track these connection and close then when needed
			go s.handleConnection(conn)
		}
	}
}

func (s *Server) serveTCPRouterClients(ctx context.Context) {
	addr := s.ServerOptions.ClientsAddr()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to start tcp listener at %s: %v ", addr, err)
	}

	log.Println("started serving tcprouter clients..")
	for {
		select {
		case <-ctx.Done():
			ln.Close()
			return

		default:
			conn, err := ln.Accept()
			if err != nil {
				log.Printf("error accepting tcprouter client connection: %v\n", err)
				continue
			}
			// TODO: need to track these connection and close then when needed
			s.handleTCPRouterClientConnection(conn)
		}
	}
}

func (s *Server) handleTCPRouterClientConnection(mainconn net.Conn) error {
	br := bufio.NewReader(mainconn)
	hs := &Handshake{}
	if err := hs.Read(br); err != nil {
		log.Printf("handshake failed")
		return err
	}
	if hs.MagicNr != MagicNr {
		return fmt.Errorf("expected %d MagicNr and received %d", MagicNr, hs.MagicNr)
	}
	log.Printf("handshake done %v", hs)
	log.Printf("Adding to active connections")
	s.activeConnections[string(hs.Secret[:])] = mainconn

	return nil
}

func (s *Server) handleConnection(mainconn net.Conn) error {
	br := bufio.NewReader(mainconn)
	serverName, isTLS, peeked := clientHelloServerName(br)
	log.Println("** SERVER NAME: SNI ", serverName, " isTLS: ", isTLS)
	return s.handleService(mainconn, serverName, peeked, isTLS)
}

func (s *Server) handleHTTPConnection(mainconn net.Conn) error {
	br := bufio.NewReader(mainconn)
	peeked := ""
	serverName := ""
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return err
		}
		peeked = peeked + line
		if strings.HasPrefix(line, "Host:") {
			serverName = strings.Trim(line[6:], " \n\r")
		}
		if strings.Trim(line, " \n\r") == "" {
			break
		}

	}
	if serverName == "" {
		return fmt.Errorf("Could not find host")
	}
	fmt.Printf("** HOST NAME: '%s'\n", serverName)
	return s.handleService(mainconn, serverName, peeked, false)
}

func (s *Server) handleService(mainconn net.Conn, serverName, peeked string, isTLS bool) error {
	serverName = strings.ToLower(serverName)
	service, exists := s.Services[serverName]
	if exists == false {
		log.Println("not found in file config, try to load it from db backend")
		var err error
		service, err = s.getHost(serverName)
		exists = err == nil
	}

	log.Println("serverName:", serverName, "exists: ", exists, " service: ", service)
	if exists == false {
		service, exists = s.Services["CATCH_ALL"]
		log.Println("using global CATCH_ALL")

		if exists == false {
			return fmt.Errorf("service doesn't exist: %v and no 'CATCH_ALL' service for request", service)
		}
		log.Println("using global CATCH_ALL service.")
	}
	var remotePort int
	if isTLS {
		remotePort = service.TLSPort
	} else {
		remotePort = service.HTTPPort
	}

	log.Println("found service: ", service)
	log.Println("handling connection from ", mainconn.RemoteAddr())

	conn := GetConn(mainconn, peeked)
	var err error
	if service.ClientSecret != "" {
		activeConn, ok := s.activeConnections[service.ClientSecret]
		if !ok {
			err = fmt.Errorf("no active connection for %s", serverName)
		} else {
			err = s.forwardConnection(conn, activeConn)
		}
	} else {
		err = s.forwardConnectionToService(conn, service.Addr, remotePort)
	}
	if err != nil {
		log.Printf("failed to forward traffic: %v\n", err)
	}
	return err

}

func (s *Server) forwardConnection(conn1, conn2 net.Conn) error {
	errChan := make(chan error, 1)
	go connCopy(conn1, conn2, errChan)
	go connCopy(conn2, conn1, errChan)

	err := <-errChan
	if err != nil {
		return fmt.Errorf("Error during connection: %v", err)
	}
	return nil
}

func (s *Server) forwardConnectionToService(conn net.Conn, remoteAddr string, remotePort int) error {
	remoteTCPAddr := &net.TCPAddr{IP: net.ParseIP(remoteAddr), Port: remotePort}
	defer conn.Close()

	connService, err := net.DialTCP("tcp", nil, remoteTCPAddr)
	if err != nil {
		return fmt.Errorf("error while connection to service: %v", err)
	}
	log.Printf("connected to the service %s\n", remoteTCPAddr.String())

	defer connService.Close()

	return s.forwardConnection(conn, connService)
}
