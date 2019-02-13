all: lint check build install

lint:
	go build -o ./tmp/golint ./vendor/golang.org/x/lint/golint && go build -o ./tmp/errcheck ./vendor/github.com/kisielk/errcheck && \
	./tmp/golint -set_exit_status $(go list ./... | grep -v /vendor/) && \
	./tmp/errcheck -ignoretests ./...

check:
	go test -v ./...

build:
	go build ./...

install:
	go install ./...

.PHONY: all lint check build install
