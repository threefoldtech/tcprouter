server:
	cd cmd/server && go build && mkdir -p ../../bin && mv server ../../bin/tcprouterserver

client:
	cd cmd/client && go build && mkdir -p ../../bin && mv client  ../../bin/tcprouterclient

all: server client

runserver: server
	sudo ./bin/tcprouterserver -config router.toml