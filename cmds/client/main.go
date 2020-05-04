package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/cenkalti/backoff/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/threefoldtech/tcprouter"
	"github.com/urfave/cli/v2"
)

func main() {
	app := cli.NewApp()
	app.Version = "0.0.1"
	app.Usage = "TCP router client"
	app.EnableBashCompletion = true
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:    "secret",
			Usage:   "secret to identify the connection",
			EnvVars: []string{"TRC_SECRET"},
		},
		&cli.StringSliceFlag{
			Name:    "remote",
			Usage:   "address to the TCP router server, this flag can be used multiple time to connect to multiple server",
			EnvVars: []string{"TRC_REMOTE"},
		},
		&cli.StringFlag{
			Name:    "local",
			Usage:   "address to the local application",
			EnvVars: []string{"TRC_LOCAL"},
		},
		&cli.StringFlag{
			Name:    "local-tls",
			Usage:   "address to the local tls application",
			EnvVars: []string{"TRC_LOCAL"},
		},
		&cli.IntFlag{
			Name:    "backoff",
			Value:   5,
			Usage:   "backoff in second",
			EnvVars: []string{"TRC_BACKOFF"},
		},
	}
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	app.Action = func(c *cli.Context) error {
		remotes := c.StringSlice("remote")
		local := c.String("local")
		localtls := c.String("local-tls")
		if len(localtls) == 0 {
			localtls = local
		}
		backoff := c.Int("backoff")
		secret := c.String("secret")

		cSig := make(chan os.Signal, 1)
		signal.Notify(cSig, os.Interrupt, syscall.SIGTERM)

		wg := sync.WaitGroup{}
		wg.Add(len(remotes))

		ctx, cancel := context.WithCancel(context.Background())

		for _, remote := range remotes {
			c := connection{
				Secret:   secret,
				Remote:   remote,
				Local:    local,
				LocalTLS: localtls,
				Backoff:  backoff,
			}
			go func() {
				defer func() {
					wg.Done()
					log.Info().Msgf("connection to %s stopped", c.Remote)
				}()
				start(ctx, c)
			}()
		}

		<-cSig
		log.Info().Msg("exit signal received, stopping")
		cancel()
		wg.Wait()

		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal().Msg(err.Error())
	}
}

type connection struct {
	Secret   string
	Remote   string
	Local    string
	LocalTLS string
	Backoff  int
}

func start(ctx context.Context, c connection) {
	client := tcprouter.NewClient(c.Secret, c.Local, c.LocalTLS, c.Remote)

	op := func() error {
		for {

			select {
			case <-ctx.Done():
				log.Info().Msg("context canceled, stopping")
				return nil

			default:
				if err := client.Start(ctx); err != nil {
					log.Error().Err(err).Send()
					return err
				}
			}
		}
	}

	bo := backoff.NewConstantBackOff(time.Second * time.Duration(c.Backoff))
	notify := func(err error, d time.Duration) {
		log.Error().Err(err).Msgf("retry in %s", d)
	}

	if err := backoff.RetryNotify(op, bo, notify); err != nil {
		log.Fatal().Err(err).Send()
	}
}
