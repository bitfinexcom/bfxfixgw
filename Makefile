all: lint check build install

lint:
	golint -set_exit_status $(go list ./... | grep -v /vendor/)

check:
	go test -v ./...

build:
	go build ./...

install:
	go install ./...

.PHONY: all lint check build install
