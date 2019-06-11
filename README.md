# tcprouter

a down to earth tcp router based on traefik tcp streaming and supports multiple backends using [valkyrie](github.com/abronan/valkeyrie)


## example

This example can be found in [examples/main.go](./examples/main.go)
```go

	kv.Put("router/register/google", []byte("google"), nil)
	kv.Put("router/backend/google/sni", []byte("www.google.com"), nil)
	kv.Put("router/backend/google/addr", []byte("172.217.19.46:443"), nil)


	kv.Put("router/register/bing", []byte("bing"), nil)
	kv.Put("router/backend/bing/sni", []byte("www.bing.com"), nil)
	kv.Put("router/backend/bing/addr", []byte("13.107.21.200:443"), nil)

```
For every backend we are proxying to you need to define some keys
- backend using `/router/register/BACKEND` = `BACKEND` 
- SNI `service name indicator` so our router can figure out which backend to forward the traffic to. and that's specified using `/router/backend/BACKEND/sni` 
- addr `tcp endpoint` usually the `ip` and `443`

If you want to test that locally you can modify `/etc/hosts`
```
127.0.0.1 www.google.com
127.0.0.1 www.bing.com
```
So your browser go to your loopback on requesting google or bing.

## Running the router

configfile: router.toml
```go
[server]
addr = "0.0.0.0"
port = 443

[server.dbbackend]
type 	 = "redis"
addr     = "127.0.0.1"
port     = 6379

```
then 
`./tcprouter router.toml`


Please notice if you are using low numbered port like 80 or 443 you can use sudo or setcap before running the binary.
- `sudo ./tcprouter router.toml`
- setcap: `sudo setcap CAP_NET_BIND_SERVICE=+eip PATH_TO_TCPROUTER`