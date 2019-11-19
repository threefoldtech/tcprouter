all: client server

server:
	mkdir -p bin
	cd cmds/server && go build && mv server ../../bin/trs

client:
	mkdir -p bin
	cd cmds/client && go build && mv client ../../bin/trc

runserver: server
	sudo bin/trs -config router.toml

.PHONY: server client 
