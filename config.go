package tcprouter

import (
	"fmt"

	"github.com/abronan/valkeyrie/store"
)

var validBackends = map[string]store.Backend{
	"redis":  store.REDIS,
	"boltdb": store.BOLTDB,
	"etcd":   store.ETCDV3,
}

// Config hold the server configuration
type Config struct {
	Server ServerConfig `toml:"server"`
}

// ServerConfig configures the server listeners and backend
type ServerConfig struct {
	Host        string             `toml:"addr"`
	Port        uint               `toml:"port"`
	HTTPPort    uint               `toml:"httpport"`
	ClientsPort uint               `toml:"clientsport"`
	DbBackend   DbBackendConfig    `toml:"dbbackend"`
	Services    map[string]Service `toml:"services"`
}

// Addr returns the listenting address of the server
func (s ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// Service defines a proxy configuration
type Service struct {
	Addr         string `toml:"addr"`
	ClientSecret string `toml:"clientsecret` // will forward connection to it directly instead of hitting the Addr.
	TLSPort      int    `toml:"tlsport"`
	HTTPPort     int    `toml:"httpport"`
}

// DbBackendConfig define the connection to a backend store
type DbBackendConfig struct {
	DbType   string `toml:"type"`
	Host     string `toml:"addr"`
	Port     uint   `toml:"port"`
	Username string `toml:"username"`
	Password string `toml:"password"`
	Token    string `toml:"token"`
	Refresh  uint   `toml:"refresh"`
	//Bucket string `toml:"bucket"`
}

// Addr returns the listenting address of the server
func (b DbBackendConfig) Addr() string {
	return fmt.Sprintf("%s:%d", b.Host, b.Port)
}

// Backend return the Backend object of the b.DbType
func (b DbBackendConfig) Backend() store.Backend {
	backend, ok := validBackends[b.DbType]
	if !ok {
		panic(fmt.Sprintf("unsupported backend type '%s'", b.DbType))
	}
	return backend
}
