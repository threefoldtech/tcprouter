package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"

	//"github.com/abronan/valkeyrie/store/boltdb"
	//"github.com/abronan/valkeyrie/store/etcd/v2"
	"log"
	"net"
	"os"
	"strconv"
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

type TCPService struct {
	Name string `json:"name"`
	Addr string `json:"addr"`
	SNI  string `json:"sni"`
}

func init() {
	if len(os.Args) != 2 {
		fmt.Println("need to pass config file.")
		os.Exit(4)
	}
	configFilePath := os.Args[1]
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

type Backend struct {
	Addr string
	Port int
}

type ServerOptions struct {
	listeningAddr string
	listeningPort uint
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
	defer s.backendM.Unlock()
	s.Backends[name] = Backend{Addr: remoteAddr, Port: port}
}

func (s *Server) DeleteBackend(name string) {
	s.backendM.Lock()
	defer s.backendM.Unlock()
	delete(s.Backends, name)
}
func (s *Server) monitorDbForBackends() {
	for {
		backendPairs, err := s.DbStore.List("tcprouter/service/", nil)
		// fmt.fmt.Println("backendPairs", backendPairs, " err: ", err)
		fmt.Println(err)
		fmt.Println(len(backendPairs))
		for _, backendPair := range backendPairs {
			tcpService := &TCPService{}
			err = json.Unmarshal(backendPair.Value, tcpService)
			if err != nil {
				fmt.Println("invalid service content.")
			}
			fmt.Println(tcpService)
			//backendName := tcpService.Name
			backendSNI := tcpService.SNI
			backendAddr := tcpService.Addr
			parts := strings.Split(string(backendAddr), ":")
			addr, portStr := parts[0], parts[1]
			port, err := strconv.Atoi(portStr)
			if err != nil {
				continue
			}
			s.RegisterBackend(backendSNI, addr, port)

		}
		fmt.Println("BACKENDS: ", s.Backends)
		time.Sleep(time.Duration(routerConfig.Server.DbBackend.Refresh) * time.Second)
	}

}

func (s *Server) Start() {
	go s.monitorDbForBackends()

	var ln net.Listener
	var err error
	ln, err = net.Listen("tcp", fmt.Sprintf("%s:%d", s.ServerOptions.listeningAddr, s.ServerOptions.listeningPort))

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

	conn := GetConn(mainconn, peeked)
	serverName = strings.ToLower(serverName)

	s.backendM.Lock()
	defer s.backendM.Unlock()
	backend, exists := s.Backends[serverName]
	// fmt.Println("serverName:", serverName, "exists: ", exists, " backend: ", backend)
	if exists == false {
		backend, exists = s.Backends["CATCH_ALL"]
		fmt.Println("using global CATCH_ALL")
		// fmt.Println("serverName:", serverName, "exists: ", exists, "backend: ", backend)

		if exists == false {
			return fmt.Errorf("backend doesn't exist: %s and no 'CATCH_ALL' backend for request.", backend)

		} else {
			fmt.Println("using global CATCH_ALL backend.")
		}
	}
	// remoteAddr := fmt.Sprintf("%s:%d",s.ServerOptions.remoteAddr,  s.ServerOptions.remotePort)
	remoteAddr := &net.TCPAddr{IP: net.ParseIP(backend.Addr), Port: backend.Port}
	fmt.Println("found backend: ", remoteAddr)
	fmt.Println("handling connection from ", conn.RemoteAddr())
	defer conn.Close()

	connBackend, err := net.DialTCP("tcp", nil, remoteAddr)
	if err != nil {
		fmt.Println("error while connection to backend: %v", err)
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

	serverOpts := ServerOptions{listeningAddr: routerConfig.Server.Addr, listeningPort: routerConfig.Server.Port}
	s := NewServer(serverOpts)
	s.DbStore = kv
	s.Backends = make(map[string]Backend)

	s.Start()

}
