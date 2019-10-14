package main

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

type tomlConfig struct {
	Server ServerConfig `toml:"server"`
}
type ServerConfig struct {
	Addr      string             `toml:"addr"`
	Port      uint               `toml:"port"`
	HttpPort  uint               `toml:"httpport"`
	DbBackend DbBackendConfig    `toml:"dbbackend"`
	Services  map[string]Service `toml:"services"`
}
type Service struct {
	Addr     string `toml:"addr"`
	TlsPort  int    `toml:"tlsport"`
	HttpPort int    `toml:"httpport"`
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

func ParseCfg(cfg string) (tomlConfig, error) {
	var conf tomlConfig

	if _, err := toml.Decode(cfg, &conf); err != nil {
		fmt.Println("couldnt parse, err: ", err)
		return conf, err
	} else {

		fmt.Println("parsed.")
		fmt.Println("Server: ", conf.Server)
		return conf, nil
	}

}
