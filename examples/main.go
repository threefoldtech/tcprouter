package main

import (
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

	kv.Put("router/register/google", []byte("google"), nil)
	kv.Put("router/backend/google/sni", []byte("www.google.com"), nil)
	kv.Put("router/backend/google/addr", []byte("172.217.19.46:443"), nil)


	kv.Put("router/register/bing", []byte("bing"), nil)
	kv.Put("router/backend/bing/sni", []byte("www.bing.com"), nil)
	kv.Put("router/backend/bing/addr", []byte("13.107.21.200:443"), nil)


}


