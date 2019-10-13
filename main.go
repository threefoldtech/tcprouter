package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"

	//"github.com/abronan/valkeyrie/store/boltdb"
	//"github.com/abronan/valkeyrie/store/etcd/v2"
	"flag"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/abronan/valkeyrie"
	"github.com/abronan/valkeyrie/store"
	"github.com/abronan/valkeyrie/store/redis"
	//"github.com/abronan/valkeyrie/store/boltdb"
	//etcd "github.com/abronan/valkeyrie/store/etcd/v3"
)

const (
	defaultRefresh = 30 * time.Second
)

var validBackends = map[string]store.Backend{
	"redis":  store.REDIS,
	"boltdb": store.BOLTDB,
	"etcd":   store.ETCDV3,
}
var routerConfig tomlConfig

func init() {
	var configFilePath string
	flag.StringVar(&configFilePath, "config", "", "Configuration file path")
	flag.Parse()
	fmt.Println("reading config from : ", configFilePath)
	bytes, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		fmt.Println("Error reading config file ", configFilePath, " err: ", err)
	}
	cfg := string(bytes)

	redis.Register()
	//boltdb.Register()
	//etcd.Register()

	c, err := ParseCfg(cfg)
	if err != nil {
		fmt.Println("invalid toml. cfg: ", cfg)
		os.Exit(2)
	}

	_, exists := validBackends[c.Server.DbBackend.DbType]
	if !exists {
		fmt.Println("invalid dbbackend type: ", c.Server.DbBackend.DbType)
		os.Exit(3)
	}
	routerConfig = c
	fmt.Println("routerConfig: ", routerConfig)
}

type ServerOptions struct {
	listeningAddr     string
	listeningTlsPort  uint
	listeningHttpPort uint
}

type Server struct {
	ServerOptions ServerOptions
	DbStore       store.Store
	Services      map[string]Service
	backendM      sync.RWMutex
}

func NewServer(forwardOptions ServerOptions) *Server {
	return &Server{ServerOptions: forwardOptions}
}

func (s *Server) RegisterService(name, remoteAddr string, tlsport int, httpport int) {
	fmt.Println("register ", name, remoteAddr, tlsport, httpport)
	s.backendM.Lock()
	defer s.backendM.Unlock()
	s.Services[name] = Service{Addr: remoteAddr, TlsPort: tlsport, HttpPort: httpport}
}

func (s *Server) DeleteService(name string) {
	s.backendM.Lock()
	defer s.backendM.Unlock()
	delete(s.Services, name)
}

func (s *Server) getHost(host string, service *Service) error {
	key := fmt.Sprintf("tcprouter/service/%s", host)
	servicePair, err := s.DbStore.Get(key, nil)
	if err != nil {
		fmt.Println("host not found", key, err)
		return fmt.Errorf("host not found")
	}
	err = json.Unmarshal(servicePair.Value, service)
	if err != nil {
		fmt.Println("invalid service content.")
		return fmt.Errorf("Invalid service content")
	}
	fmt.Println(service)
	return nil
}

func (s *Server) RegisterServicesFromConfig() {
	for key, service := range routerConfig.Server.Services {
		s.RegisterService(key, service.Addr, service.TlsPort, service.HttpPort)
	}
}

func (s *Server) monitorServerConfigFile() {

}

func (s *Server) serveHTTP() {
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", s.ServerOptions.listeningAddr, s.ServerOptions.listeningHttpPort))

	if err != nil {
		fmt.Println("err: ", err)
		// handle error
	}

	fmt.Println("started http server..")
	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("err")
			// handle error
		}
		go s.handleHTTPConnection(conn)
	}

}

func (s *Server) Start() {
	s.RegisterServicesFromConfig()
	go s.monitorServerConfigFile()
	go s.serveHTTP()

	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", s.ServerOptions.listeningAddr, s.ServerOptions.listeningTlsPort))

	if err != nil {
		fmt.Println("err: ", err)
		// handle error
	}

	fmt.Println("started server..")
	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("err")
			// handle error
		}
		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(mainconn net.Conn) error {
	br := bufio.NewReader(mainconn)
	serverName, isTls, peeked := clientHelloServerName(br)
	fmt.Println("** SERVER NAME: SNI ", serverName, " isTLS: ", isTls)
	return s.handleService(mainconn, serverName, peeked, isTls)
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

func (s *Server) handleService(mainconn net.Conn, serverName, peeked string, isTls bool) error {
	serverName = strings.ToLower(serverName)
	service, exists := s.Services[serverName]
	if exists == false {
		fmt.Println("not found in file config")
		// try to load it from db backend
		service = Service{}
		err := s.getHost(serverName, &service)
		exists = err == nil
	}

	fmt.Println("serverName:", serverName, "exists: ", exists, " service: ", service)
	if exists == false {
		service, exists = s.Services["CATCH_ALL"]
		fmt.Println("using global CATCH_ALL")

		if exists == false {
			return fmt.Errorf("service doesn't exist: %s and no 'CATCH_ALL' service for request.", service)

		} else {
			fmt.Println("using global CATCH_ALL service.")
		}
	}
	var remotePort int
	if isTls {
		remotePort = service.TlsPort
	} else {
		remotePort = service.HttpPort
	}
	fmt.Println("found service: ", service)
	fmt.Println("handling connection from ", mainconn.RemoteAddr())
	conn := GetConn(mainconn, peeked)
	return s.forward(conn, service.Addr, remotePort)

}

func (s *Server) forward(conn net.Conn, remoteAddr string, remotePort int) error {
	remoteTCPAddr := &net.TCPAddr{IP: net.ParseIP(remoteAddr), Port: remotePort}
	defer conn.Close()

	connService, err := net.DialTCP("tcp", nil, remoteTCPAddr)
	if err != nil {
		fmt.Println("error while connection to service: %v", err)
		return err
	} else {
		fmt.Println("connected to the service...")
	}
	defer connService.Close()

	errChan := make(chan error, 1)
	go connCopy(conn, connService, errChan)
	go connCopy(connService, conn, errChan)

	err = <-errChan
	if err != nil {
		fmt.Println("Error during connection: %v", err)
		return err
	}
	return nil
}

func main() {
	fmt.Println("main config: ", routerConfig)
	kvStore, _ := validBackends[routerConfig.Server.DbBackend.DbType] // at this point backend exists or the app would have exited.
	// Initialize a new store with dbbackendtype
	kv, err := valkeyrie.NewStore(
		kvStore,
		[]string{fmt.Sprintf("%s:%d", routerConfig.Server.DbBackend.Addr, routerConfig.Server.DbBackend.Port)},
		&store.Config{
			ConnectionTimeout: 10 * time.Second,
		},
	)
	if err != nil {
		log.Fatal("Cannot create store redis", err)
	}

	serverOpts := ServerOptions{listeningAddr: routerConfig.Server.Addr, listeningTlsPort: routerConfig.Server.Port, listeningHttpPort: routerConfig.Server.HttpPort}
	s := NewServer(serverOpts)
	s.DbStore = kv
	s.Services = make(map[string]Service)

	s.Start()

}
