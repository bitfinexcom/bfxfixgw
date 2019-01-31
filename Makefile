all: lint check build install

lint:
	go run ./vendor/golang.org/x/lint/golint -set_exit_status $(go list ./... | grep -v /vendor/) && \
	echo hello

check:
	go test -v ./...

build:
	go build ./...

install:
	go install ./...

.PHONY: all lint check build install
