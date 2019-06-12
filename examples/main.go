package main

import (
	"encoding/json"
	"log"
	"time"

	"github.com/abronan/valkeyrie"
	"github.com/abronan/valkeyrie/store"
	//"github.com/abronan/valkeyrie/store/boltdb"
	//etcd "github.com/abronan/valkeyrie/store/etcd/v3"
	"github.com/abronan/valkeyrie/store/redis"
)
var validBackends = map[string]store.Backend{
	"redis": store.REDIS,
	//"boltdb": store.BOLTDB,
	//"etcd": store.ETCDV3,

}

func init() {

	redis.Register()
	//boltdb.Register()
	//etcd.Register()

}

type Service struct {
	Addr string `json:"addr"`
	SNI  string `json:"sni"`
	Name string `json:"bing"`
}

func main() {

	// Initialize a new store with redis
	kv, err := valkeyrie.NewStore(
		store.REDIS,
		[]string{"127.0.0.1:6379" },
		&store.Config{
			ConnectionTimeout: 10 * time.Second,
		},
	)
	if err != nil {
		log.Fatal("Cannot create store redis")
	}
	google := &Service{Addr:"172.217.19.46:443", SNI:"www.google.com", Name:"google"}
	encGoogle, _ := json.Marshal(google)
	bing := &Service{Addr:"13.107.21.200:443", SNI:"www.bing.com", Name:"bing"}
	encBing, _ := json.Marshal(bing)

	kv.Put("/tcprouter/service/google", encGoogle, nil)
	kv.Put("/tcprouter/service/bing", encBing, nil)


}


