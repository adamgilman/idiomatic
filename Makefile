.PHONY: build test vet lint install clean integration-test

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  = -X github.com/adamgilman/idiomatic/cmd/idio/cmd.Version=$(VERSION) \
           -X github.com/adamgilman/idiomatic/cmd/idio/cmd.Commit=$(COMMIT) \
           -X github.com/adamgilman/idiomatic/cmd/idio/cmd.Date=$(DATE)

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o dist/idio ./cmd/idio

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

install: build
	mkdir -p $(HOME)/.local/bin
	cp dist/idio $(HOME)/.local/bin/idio

clean:
	rm -rf dist/

integration-test: build
	bash testing/run-integration-tests.sh
