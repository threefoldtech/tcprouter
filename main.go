package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abronan/valkeyrie"
	"github.com/abronan/valkeyrie/store"
	"github.com/abronan/valkeyrie/store/redis"
)

func init() {

	redis.Register()
}

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
	DbStore       store.Store
	Backends      map[string]Backend
	backendM      sync.RWMutex
}

func NewServer(forwardOptions ServerOptions) *Server {
	return &Server{ServerOptions: forwardOptions}
}

func (s *Server) RegisterBackend(name, remoteAddr string, port int) {
	fmt.Println("register ", name, remoteAddr, port)
	s.backendM.Lock()
	s.Backends[name] = Backend{Addr: remoteAddr, Port: port}
	s.backendM.Unlock()
}

func (s *Server) DeleteBackend(name string) {
	s.backendM.Lock()
	delete(s.Backends, name)
	s.backendM.Unlock()

}
func (s *Server) monitorDbForBackends() {

	for {

		backendPairs, err := s.DbStore.List("router/register/", nil)
		// fmt.fmt.Println("backendPairs", backendPairs, " err: ", err)
		fmt.Println(err)
		fmt.Println(len(backendPairs))
		for _, backendPair := range backendPairs {
			backendName := string(backendPair.Value)
			fmt.Println("backedname :", backendName)
			sniKey := fmt.Sprintf("router/backend/%s/sni", backendName)
			addrKey := fmt.Sprintf("router/backend/%s/addr", backendName)
			fmt.Println("sni key: ", sniKey)
			fmt.Println("addr key: ", addrKey)

			backendSNI, _ := s.DbStore.Get(sniKey, nil)
			fmt.Println(string(backendSNI.Value), err)

			backendAddr, _ := s.DbStore.Get(addrKey, nil)
			fmt.Println("*****backendAddr: ", backendAddr.Value)
			parts := strings.Split(string(backendAddr.Value), ":")
			addr, portStr := parts[0], parts[1]
			port, _ := strconv.Atoi(portStr)
			s.RegisterBackend(string(backendSNI.Value), addr, port)

		}
		time.Sleep(time.Second * 3)
	}

}

func (s *Server) Start() {

	s.DbStore.Put("router/register/server1", []byte("server1"), nil)
	s.DbStore.Put("router/backend/server1/sni", []byte("server1.com"), nil)
	s.DbStore.Put("router/backend/server1/addr", []byte("127.0.0.1:5500"), nil)

	go s.monitorDbForBackends()

	var ln net.Listener
	var err error
	ln, err = net.Listen("tcp", fmt.Sprintf("%s:%d", s.ServerOptions.listeningAddr, s.ServerOptions.listeningPort))

	if err != nil {
		fmt.Println("err: ", err)
		// handle error
	}

	fmt.Println("Started server..")
	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("err")
			// handle error
		}
		go s.handleConnection(conn)
	}
}
func main() {

	// Initialize a new store with consul
	kv, err := valkeyrie.NewStore(
		store.REDIS,
		[]string{"127.0.0.1:6379"},
		&store.Config{
			ConnectionTimeout: 10 * time.Second,
		},
	)
	if err != nil {
		log.Fatal("Cannot create store redis")
	}

	serverOpts := ServerOptions{listeningAddr: "0.0.0.0", listeningPort: 443}
	s := NewServer(serverOpts)
	s.DbStore = kv
	s.Backends = make(map[string]Backend)
	// let's make it configurable later.
	s.Backends["*"] = Backend{Addr: "127.0.0.1", Port: 9092}
	s.Backends["first.mybot.testsbots.grid.tf"] = Backend{Addr: "37.59.44.168", Port: 443}
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
func (s *Server) handleConnection(mainconn net.Conn) error {
	br := bufio.NewReader(mainconn)
	serverName, isTls, peeked := clientHelloServerName(br)
	fmt.Println("SERVER NAME: ", serverName, " isTLS: ", isTls)

	conn := GetConn(mainconn, peeked)
	serverName = strings.ToLower(serverName)

	s.backendM.Lock()
	backend, exists := s.Backends[serverName]
	if exists == false {
		backend, exists = s.Backends["*"]
		if exists == false {
			s.backendM.Unlock()
			return fmt.Errorf("backend doesn't exist: %s and no '*' backend for request.", backend)

		} else {
			fmt.Println("using global catchall backend.")
		}
	}
	s.backendM.Unlock()
	// remoteAddr := fmt.Sprintf("%s:%d",s.ServerOptions.remoteAddr,  s.ServerOptions.remotePort)
	remoteAddr := &net.TCPAddr{IP: net.ParseIP(backend.Addr), Port: backend.Port}
	fmt.Println("found backend: ", remoteAddr)
	fmt.Println("Handling connection from %s", conn.RemoteAddr())
	defer conn.Close()

	connBackend, err := net.DialTCP("tcp", nil, remoteAddr)
	if err != nil {
		fmt.Println("Error while connection to backend: %v", err)
		return err
	} else {
		fmt.Println("connected to the backend...")
	}
	defer connBackend.Close()

	errChan := make(chan error, 1)
	go connCopy(conn, connBackend, errChan)
	go connCopy(connBackend, conn, errChan)

	err = <-errChan
	if err != nil {
		fmt.Println("Error during connection: %v", err)
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
