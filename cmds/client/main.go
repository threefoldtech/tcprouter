package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/cenkalti/backoff/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/xmonader/tcprouter"
)

var (
	secret     string
	remoteAddr string
	localAddr  string
	boDuration int
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	flag.StringVar(&secret, "secret", "", "secret to identity the connection")
	flag.StringVar(&remoteAddr, "remote", "", "address to the TCP router server")
	flag.StringVar(&localAddr, "local", "", "address to the local application")
	flag.IntVar(&boDuration, "backoff", 5, "backoff in second")
	flag.Parse()

	client := tcprouter.NewClient(secret)
	op := func() error {
		for {
			log.Info().
				Str("addr", remoteAddr).
				Msg("connect to TCP router server")
			if err := client.ConnectRemote(remoteAddr); err != nil {
				return fmt.Errorf("failed to connect to TCP router server: %w", err)
			}

			log.Info().Msg("start hanshake")
			if err := client.Handshake(); err != nil {
				return fmt.Errorf("failed to hanshake with TCP router server: %w", err)
			}
			log.Info().Msg("hanshake done")

			log.Info().
				Str("addr", localAddr).
				Msg("connect to local application")
			if err := client.ConnectLocal(localAddr); err != nil {
				return fmt.Errorf("failed to connect to local application: %w", err)
			}

			log.Info().Msg("wait incoming traffic")
			client.Forward()
		}
	}

	bo := backoff.NewConstantBackOff(time.Second * time.Duration(boDuration))
	notify := func(err error, d time.Duration) {
		log.Error().Err(err).Msgf("retry in %s", d)
	}

	if err := backoff.RetryNotify(op, bo, notify); err != nil {
		log.Fatal().Err(err).Send()
	}
}
