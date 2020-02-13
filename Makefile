all: client server

BUILD_FLAGS = -ldflags '-extldflags "-fno-PIC -static"' -buildmode pie -tags 'osusergo netgo static_build'


getdeps:
	@echo "Installing golint" && go get -u golang.org/x/lint/golint
	@echo "Installing gocyclo" && go get -u github.com/fzipp/gocyclo
	@echo "Installing deadcode" && go get -u github.com/remyoudompheng/go-misc/deadcode
	@echo "Installing misspell" && go get -u github.com/client9/misspell/cmd/misspell
	@echo "Installing ineffassign" && go get -u github.com/gordonklaus/ineffassign

verifiers: vet fmt lint cyclo spelling static #deadcode

vet:
	@echo "Running $@"
	@go vet -atomic -bool -copylocks -nilfunc -printf -rangeloops -unreachable -unsafeptr -unusedresult ./...

fmt:
	@echo "Running $@"
	@gofmt -d .

lint:
	@echo "Running $@"
	golint -set_exit_status $(shell go list ./... | grep -v stubs)

ineffassign:
	@echo "Running $@"
	ineffassign .

cyclo:
	@echo "Running $@"
	gocyclo -over 100 .

deadcode:
	@echo "Running $@"
	deadcode -test $(shell go list ./...) || true

spelling:
	misspell -i monitord -error `find .`

static:
	go run honnef.co/go/tools/cmd/staticcheck -- ./...

# Builds minio, runs the verifiers then runs the tests.
check: test
test: verifiers build
	go test -v ./...

build: server client

server:
	mkdir -p bin
	cd cmds/server && go build $(BUILD_FLAGS) -o ../../bin/trs

client:
	mkdir -p bin
	cd cmds/client && go build $(BUILD_FLAGS) -o ../../bin/trc

runserver: server
	sudo bin/trs -config router.toml

.PHONY: server client 
