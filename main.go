package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"github.com/coredns/coredns/plugin/pkg/log"
	"io"
	"net"
	"strings"
	"sync"
)
type Backend struct {
	Addr string
	Port int
}

type ServerOptions struct {
	listeningAddr string
	listeningPort int
}

type Server struct {
	ServerOptions ServerOptions
	Backends      map[string]Backend
	backendM 	  sync.RWMutex
}
func NewServer(forwardOptions ServerOptions) *Server {
	return &Server{ServerOptions: forwardOptions}
}

func (s *Server) RegisterBackend(name, remoteAddr string, port int) {
	s.backendM.Lock()
	s.Backends[name] = Backend{Addr:remoteAddr, Port:port}
	s.backendM.Unlock()
}

func (s *Server) DeleteBackend(name string) {
	s.backendM.Lock()
	delete(s.Backends, name)
	s.backendM.Unlock()

}

func (s *Server) Start() {
	var ln net.Listener
	var err	error
	ln, err = net.Listen("tcp", fmt.Sprintf("%s:%d", s.ServerOptions.listeningAddr, s.ServerOptions.listeningPort))

	if err != nil {
		println("err: ", err)
		// handle error
	}

	println("Started server..")
	for {
		conn, err := ln.Accept()
		if err != nil {
			println("err")
			// handle error
		}
		go s.handleConnection(conn)
	}
}
func main() {
	serverOpts := ServerOptions{listeningAddr:"0.0.0.0", listeningPort:443}
	s := NewServer(serverOpts)
	s.Backends = make(map[string]Backend)
	// let's make it configurable later.
	s.Backends["*"] = Backend{Addr:"127.0.0.1", Port:9092}
	s.Backends["first.mybot.testsbots.grid.tf"] = Backend{Addr:"37.59.44.168", Port:443}
	s.Start()

}

// Code extracted from traefik to get the servername from TLS connection.
// GetConn creates a connection proxy with a peeked string
func GetConn(conn net.Conn, peeked string) net.Conn {
	conn = &Conn{
		Peeked: []byte(peeked),
		Conn:   conn,
	}
	return conn
}
func (s *Server) handleConnection(mainconn net.Conn) (error) {
	br := bufio.NewReader(mainconn)
	serverName, isTls, peeked := clientHelloServerName(br)
	println("SERVER NAME: ", serverName, " isTLS: ", isTls)

	conn := GetConn(mainconn, peeked)
	serverName = strings.ToLower(serverName)

	s.backendM.Lock()
	backend, exists := s.Backends[serverName]
	if exists == false {
		backend, exists = s.Backends["*"]
		if exists == false {
			s.backendM.Unlock()
			return fmt.Errorf("backend doesn't exist: %s and no '*' backend for request.", backend)

		} else{
			println("using global catchall backend.")
		}
	}
	s.backendM.Unlock()
	// remoteAddr := fmt.Sprintf("%s:%d",s.ServerOptions.remoteAddr,  s.ServerOptions.remotePort)
	remoteAddr := &net.TCPAddr{IP:net.ParseIP(backend.Addr), Port:backend.Port}
	println("found backend: ", remoteAddr)
	log.Debugf("Handling connection from %s", conn.RemoteAddr())
	defer conn.Close()

	connBackend, err := net.DialTCP("tcp", nil, remoteAddr)
	if err != nil {
		log.Errorf("Error while connection to backend: %v", err)
		return err
	}else{
		println("connected to the backend...")
	}
	defer connBackend.Close()

	errChan := make(chan error, 1)
	go connCopy(conn, connBackend, errChan)
	go connCopy(connBackend, conn, errChan)

	err = <-errChan
	if err != nil {
		log.Errorf("Error during connection: %v", err)
		return err
	}
	return nil
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
			log.Errorf("Error while Peeking first byte: %s", err)
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
		log.Errorf("Error while Peeking hello: %s", err)
		return "", false, getPeeked(br)
	}
	recLen := int(hdr[3])<<8 | int(hdr[4]) // ignoring version in hdr[1:3]
	helloBytes, err := br.Peek(recordHeaderLen + recLen)
	if err != nil {
		log.Errorf("Error while Hello: %s", err)
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
		log.Errorf("Could not get anything: %s", err)
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
	_ , err := io.Copy(dst, src)
	errCh <- err
}