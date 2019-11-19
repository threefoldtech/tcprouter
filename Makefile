server:
	cd cmd/server && go build && mv server ../../tcprouterserver

client:
	cd cmd/client && go build && mv client ../../tcprouterclient

all: server client

runserver: server
	sudo ./tcprouterserver -config router.toml