VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build install clean lint test-unit test

build:
	go build $(LDFLAGS) -o rival .

install:
	go install $(LDFLAGS) .

clean:
	rm -f rival

lint:
	golangci-lint run ./...

test-unit:
	go test -v -race ./...

test: lint test-unit build
