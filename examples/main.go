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

	kv.Put("router/register/googleeg", []byte("googleeg"), nil)
	kv.Put("router/backend/googleeg/sni", []byte("www.google.com.eg"), nil)
	kv.Put("router/backend/googleeg/addr", []byte("172.217.18.227:443"), nil)


	kv.Put("router/register/bing", []byte("bing"), nil)
	kv.Put("router/backend/bing/sni", []byte("bing.com"), nil)
	kv.Put("router/backend/bing/addr", []byte("204.79.197.200:443"), nil)


}


