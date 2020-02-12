package main

import (
	"encoding/json"
	"log"
	"time"

	"github.com/abronan/valkeyrie"
	"github.com/abronan/valkeyrie/store"
	"github.com/abronan/valkeyrie/store/redis"
)

func init() {

	redis.Register()
	//boltdb.Register()
	//etcd.Register()

}

type service struct {
	Addr string `json:"addr"`
	SNI  string `json:"sni"`
	Name string `json:"bing"`
}

func main() {

	// Initialize a new store with redis
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
	google := &service{Addr: "172.217.19.46:443", SNI: "www.google.com", Name: "google"}
	encGoogle, _ := json.Marshal(google)
	bing := &service{Addr: "13.107.21.200:443", SNI: "www.bing.com", Name: "bing"}
	encBing, _ := json.Marshal(bing)

	kv.Put("/tcprouter/service/google", encGoogle, nil)
	kv.Put("/tcprouter/service/bing", encBing, nil)

}
