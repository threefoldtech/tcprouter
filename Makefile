all: client server

BUILD_FLAGS = -ldflags '-extldflags "-fno-PIC -static"' -buildmode pie -tags 'osusergo netgo static_build'

server:
	mkdir -p bin
	cd cmds/server && go build $(BUILD_FLAGS) -o ../../bin/trs

client:
	mkdir -p bin
	cd cmds/client && go build $(BUILD_FLAGS) -o ../../bin/trc

runserver: server
	sudo bin/trs -config router.toml

.PHONY: server client 
