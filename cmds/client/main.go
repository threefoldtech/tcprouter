package main

import (
	"flag"
	"log"

	"github.com/xmonader/tcprouter"
)

var (
	secret     string
	remoteAddr string
	localAddr  string
)

func main() {
	flag.StringVar(&secret, "secret", "", "secret to identity the connection")
	flag.StringVar(&remoteAddr, "remote", "", "address to the TCP router server")
	flag.StringVar(&localAddr, "local", "", "address to the local application")
	flag.Parse()

	// // TODO: validate secret format
	// s := tcprouter.Secret(secret)
	// if err := s.Validate(); err != nil {
	// 	log.Fatalf("invalid secret format: %v", err)
	// }

	client := tcprouter.NewClient(secret)
	for {
		log.Printf("connect to TCP router server at %v", remoteAddr)
		if err := client.ConnectRemote(remoteAddr); err != nil {
			log.Fatalf("failed to connect to TCP router server %v", err)
		}

		log.Printf("start hanshake")
		if err := client.Handshake(); err != nil {
			log.Fatalf("failed to hanshake with TCP router server %v", err)
		}
		log.Printf("hanshake done")

		log.Printf("connect to local application at %v", localAddr)
		if err := client.ConnectLocal(localAddr); err != nil {
			log.Fatalf("failed to connect to local application %v", err)
		}

		log.Printf("wait incoming traffic")
		if err := client.Forward(); err != nil {
			log.Fatalf("error during forwarding %v", err)
		}

		client.Close()
	}

}
