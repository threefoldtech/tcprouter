package main

import (
	"fmt"
	"github.com/BurntSushi/toml"
)
type tomlConfig struct {
	Server ServerConfig `toml:"server"`
}
type ServerConfig struct {
	Addr string `toml:"addr"`
	Port uint	`toml:"port"`
	DbBackend DbBackendConfig `toml:"dbbackend"`
}

type DbBackendConfig struct {
	DbType string `toml:"type"`
	Addr string `toml:"addr"`
	Port uint   `toml:"port"`
	Username string `toml:"username"`
	Password string `toml:"password"`
	Token string `toml:"token"`
	//Bucket string `toml:"bucket"`

}
func ParseCfg(cfg string) (tomlConfig, error){
	var conf tomlConfig

	if _, err := toml.Decode(cfg, &conf); err != nil {
		fmt.Println("couldnt parse, err: ", err)
		return conf, err
	} else{

		fmt.Println("parsed.")
		fmt.Println("Server: ", conf.Server)
		return conf, nil
	}

}