package main

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
	listeningAddr     string
	listeningTLSPort  uint
	listeningHTTPPort uint
}

func (o ServerOptions) HTTPAddr() string {
	return fmt.Sprintf("%s:%d", o.listeningAddr, o.listeningHTTPPort)
}

func (o ServerOptions) TLSAddr() string {
	return fmt.Sprintf("%s:%d", o.listeningAddr, o.listeningTLSPort)
}

type Server struct {
	ServerOptions ServerOptions
	DbStore       store.Store
	Services      map[string]Service
	backendM      sync.RWMutex

	ctx context.Context
	wg  sync.WaitGroup
}

func NewServer(forwardOptions ServerOptions, store store.Store, services map[string]Service) *Server {
	if services == nil {
		services = make(map[string]Service)
	}

	return &Server{
		ServerOptions: forwardOptions,
		Services:      services,
		DbStore:       store,
	}
}

func (s *Server) Start(ctx context.Context) error {
	for key, service := range s.Services {
		s.RegisterService(key, service.Addr, service.TLSPort, service.HTTPPort)
	}

	go s.serveHTTP(ctx)
	go s.serveTLS(ctx)

	<-ctx.Done()

	log.Print("stopping server...")
	s.wg.Wait()
	log.Println("stopped")

	return nil
}

func (s *Server) RegisterService(name, remoteAddr string, tlsport int, httpport int) {
	log.Println("register ", name, remoteAddr, tlsport, httpport)
	s.backendM.Lock()
	defer s.backendM.Unlock()
	s.Services[name] = Service{Addr: remoteAddr, TLSPort: tlsport, HTTPPort: httpport}
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
			return fmt.Errorf("service doesn't exist: %s and no 'CATCH_ALL' service for request", service)
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
	if err := s.forward(conn, service.Addr, remotePort); err != nil {
		log.Printf("failed to forward traffic: %v\n", err)
	}
	return nil
}

func (s *Server) forward(conn net.Conn, remoteAddr string, remotePort int) error {
	remoteTCPAddr := &net.TCPAddr{IP: net.ParseIP(remoteAddr), Port: remotePort}
	defer conn.Close()

	connService, err := net.DialTCP("tcp", nil, remoteTCPAddr)
	if err != nil {
		return fmt.Errorf("error while connection to service: %w", err)
	}
	log.Printf("connected to the service %s\n", remoteTCPAddr.String())

	defer connService.Close()

	errChan := make(chan error, 1)
	go connCopy(conn, connService, errChan)
	go connCopy(connService, conn, errChan)

	err = <-errChan
	if err != nil {
		return fmt.Errorf("Error during connection: %w", err)
	}
	return nil
}
