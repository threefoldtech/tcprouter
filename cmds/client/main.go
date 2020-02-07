package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
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
			Name:  "secret",
			Usage: "secret to identify the connection",
		},
		&cli.StringSliceFlag{
			Name:  "remote",
			Usage: "address to the TCP router server, this flag can be used multiple time to connect to multiple server",
		},
		&cli.StringFlag{
			Name:  "local",
			Usage: "address to the local application",
		},
		&cli.IntFlag{
			Name:  "backoff",
			Value: 5,
			Usage: "backoff in second",
		},
	}
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	app.Action = func(c *cli.Context) error {
		remotes := c.StringSlice("remote")
		local := c.String("local")
		backoff := c.Int("backoff")
		secret := c.String("secret")

		cSig := make(chan os.Signal)
		signal.Notify(cSig, os.Interrupt, os.Kill)

		for _, remote := range remotes {
			c := connection{
				Secret:  secret,
				Remote:  remote,
				Local:   local,
				Backoff: backoff,
			}
			go func() {
				start(context.TODO(), c)
			}()
		}

		<-cSig

		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal().Msg(err.Error())
	}
}

type connection struct {
	Secret  string
	Remote  string
	Local   string
	Backoff int
}

func start(ctx context.Context, c connection) {
	client := tcprouter.NewClient(c.Secret)

	op := func() error {
		for {

			select {
			case <-ctx.Done():
				log.Info().Msg("context canceled, stopping")
				return nil

			default:

				log.Info().
					Str("addr", c.Remote).
					Msg("connect to TCP router server")
				if err := client.ConnectRemote(c.Remote); err != nil {
					return fmt.Errorf("failed to connect to TCP router server: %w", err)
				}

				log.Info().Msg("start hanshake")
				if err := client.Handshake(); err != nil {
					return fmt.Errorf("failed to hanshake with TCP router server: %w", err)
				}
				log.Info().Msg("hanshake done")

				log.Info().
					Str("addr", c.Local).
					Msg("connect to local application")
				if err := client.ConnectLocal(c.Local); err != nil {
					return fmt.Errorf("failed to connect to local application: %w", err)
				}

				log.Info().Msg("wait incoming traffic")
				client.Forward()
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
