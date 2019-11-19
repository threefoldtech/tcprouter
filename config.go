package tcprouter

import (
	"fmt"
	"io"

	"github.com/BurntSushi/toml"
	"github.com/abronan/valkeyrie/store"
)

var validBackends = map[string]store.Backend{
	"redis":  store.REDIS,
	"boltdb": store.BOLTDB,
	"etcd":   store.ETCDV3,
}

type Config struct {
	Server ServerConfig `toml:"server"`
}

type ServerConfig struct {
	Host        string             `toml:"addr"`
	Port        uint               `toml:"port"`
	HTTPPort    uint               `toml:"httpport"`
	ClientsPort uint               `toml:"clientsport"`
	DbBackend   DbBackendConfig    `toml:"dbbackend"`
	Services    map[string]Service `toml:"services"`
}

func (s ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

type Service struct {
	Addr     string `toml:"addr"`
	TLSPort  int    `toml:"tlsport"`
	HTTPPort int    `toml:"httpport"`
}

type DbBackendConfig struct {
	DbType   string `toml:"type"`
	Addr     string `toml:"addr"`
	Port     uint   `toml:"port"`
	Username string `toml:"username"`
	Password string `toml:"password"`
	Token    string `toml:"token"`
	Refresh  uint   `toml:"refresh"`
	//Bucket string `toml:"bucket"`
}

func (b DbBackendConfig) Backend() store.Backend {
	backend, ok := validBackends[b.DbType]
	if !ok {
		panic(fmt.Sprintf("unsupported backend type '%s'", b.DbType))
	}
	return backend
}

func ParseCfg(r io.Reader) (Config, error) {
	var conf Config
	_, err := toml.DecodeReader(r, &conf)
	return conf, err
}
