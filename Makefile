BIN      := bin
PKG      := github.com/opengfx1030/rdnactl
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X $(PKG)/internal/cli.Version=$(VERSION)
GOFLAGS  := -trimpath

.PHONY: build linux test lint check fmt clean

build:
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BIN)/rdnactl ./cmd/rdnactl

linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BIN)/rdnactl-linux-amd64 ./cmd/rdnactl

test:
	go test ./...

lint:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	go vet ./...

check: lint test

fmt:
	gofmt -w .

clean:
	rm -rf $(BIN)
