package main

import (
	"context"
	"fmt"
	"os/signal"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"

	"os"
	"time"

	"github.com/abronan/valkeyrie"
	"github.com/abronan/valkeyrie/store"
	"github.com/abronan/valkeyrie/store/redis"
	"github.com/threefoldtech/tcprouter"
)

var validBackends = map[string]store.Backend{
	"redis":  store.REDIS,
	"boltdb": store.BOLTDB,
	"etcd":   store.ETCDV3,
}

func readConfig(path string) (tcprouter.Config, error) {
	f, err := os.Open(path)
	if err != nil {
		log.Fatal().
			Str("path", path).
			Err(err).
			Msg("failed to open configuration file")
	}
	defer f.Close()

	c, err := tcprouter.ParseCfg(f)
	if err != nil {
		return c, fmt.Errorf("failed to read configuration %w", err)
	}
	return c, nil
}

func initBackend(cfg tcprouter.Config) error {
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
	app := cli.NewApp()
	app.Version = "0.0.1"
	app.Usage = "TCP router client"
	app.EnableBashCompletion = true
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:  "config",
			Usage: "Path to configuration file",
			Value: "config.toml",
		},
	}
	app.Action = func(c *cli.Context) error {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
		cfgPath := c.String("config")
		log.Printf("reading config from: %v", cfgPath)
		cfg, err := readConfig(cfgPath)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to read configuration")
		}
		log.Printf("main config: %+v", cfg)

		if err := initBackend(cfg); err != nil {
			log.Fatal().Err(err).Msg("failed to  initialize database backend")
		}

		backend := cfg.Server.DbBackend.Backend()
		addr := cfg.Server.DbBackend.Addr()
		kv, err := initStore(backend, addr)
		if err != nil {
			log.Fatal().
				Err(err).
				Str("backend type", string(backend)).
				Msg("Cannot create backend store")
		}

		serverOpts := tcprouter.ServerOptions{
			ListeningAddr:           cfg.Server.Host,
			ListeningTLSPort:        cfg.Server.Port,
			ListeningHTTPPort:       cfg.Server.HTTPPort,
			ListeningForClientsPort: cfg.Server.ClientsPort,
		}
		s := tcprouter.NewServer(serverOpts, kv, cfg.Server.Services)

		cSig := make(chan os.Signal, 1)
		signal.Notify(cSig, os.Interrupt, os.Kill)

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			// Block until a signal is received.
			<-cSig
			cancel()
		}()

		return s.Start(ctx)
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal().Msg(err.Error())
	}
}
