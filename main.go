package main

import (
	"context"
	"flag"
	"fmt"
	"os/signal"

	"log"
	"os"
	"time"

	"github.com/abronan/valkeyrie"
	"github.com/abronan/valkeyrie/store"
	"github.com/abronan/valkeyrie/store/redis"
)

var validBackends = map[string]store.Backend{
	"redis":  store.REDIS,
	"boltdb": store.BOLTDB,
	"etcd":   store.ETCDV3,
}

func readConfig(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("failed to open configuration file at %s: %v", path, err)
	}
	defer f.Close()

	c, err := ParseCfg(f)
	if err != nil {
		return c, fmt.Errorf("failed to read configuration %w", err)
	}
	return c, nil
}

func initBackend(cfg Config) error {
	redis.Register()

	_, exists := validBackends[cfg.Server.DbBackend.DbType]
	if !exists {
		return fmt.Errorf("invalid db backend type %s", cfg.Server.DbBackend.DbType)
	}

	return nil
}

func initStore(backend store.Backend, addr string) (store.Store, error) {
	return valkeyrie.NewStore(
		backend,
		[]string{addr},
		&store.Config{
			ConnectionTimeout: 10 * time.Second,
		},
	)
}

var (
	cfgPath string
)

func main() {

	flag.StringVar(&cfgPath, "config", "", "Configuration file path")
	flag.Parse()

	log.Printf("reading config from: %v", cfgPath)
	cfg, err := readConfig(cfgPath)
	if err != nil {
		log.Fatalf("failed to read configuration: %v", err)
	}
	log.Printf("main config: %+v", cfg)

	if err := initBackend(cfg); err != nil {
		log.Fatalf("failed to  initialize database backend: %v", err)
	}

	backend := cfg.Server.DbBackend.Backend()
	addr := cfg.Server.Addr()
	kv, err := initStore(backend, addr)
	if err != nil {
		log.Fatalf("Cannot create %s store: %v", backend, err)
	}

	serverOpts := ServerOptions{
		listeningAddr:     cfg.Server.Host,
		listeningTLSPort:  cfg.Server.Port,
		listeningHTTPPort: cfg.Server.HTTPPort,
	}
	s := NewServer(serverOpts, kv, cfg.Server.Services)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		// Block until a signal is received.
		<-c
		cancel()
	}()

	s.Start(ctx)

}
