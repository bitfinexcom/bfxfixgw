all: lint check build install

lint:
	go build -o ./tmp/golangci-lint ./vendor/github.com/golangci/golangci-lint/cmd/golangci-lint && \
	./tmp/golangci-lint run

check:
	go test -v ./...

build:
	go build ./...

install:
	go install ./...

.PHONY: all lint check build install
