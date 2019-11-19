server:
	cd cmd/server && go build && cp server ../../tcprouterserver

client:
	cd cmd/client && go build && cp client ../../tcprouterclient

all: server client

runserver: server
	sudo ./tcprouterserver -config router.toml