BINARY  := mcp-setup
MODULE  := github.com/rhuss/mcp-setup
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
GOFLAGS := -buildvcs=false

.PHONY: build install clean cross-compile

build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY) .

install: build
	install -m 755 $(BINARY) $(HOME)/.local/bin/$(BINARY)

clean:
	rm -f $(BINARY) $(BINARY)-*

cross-compile:
	GOOS=darwin GOARCH=arm64 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY)-darwin-arm64 .
	GOOS=darwin GOARCH=amd64 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY)-darwin-amd64 .
	GOOS=linux  GOARCH=amd64 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY)-linux-amd64 .
	GOOS=linux  GOARCH=arm64 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY)-linux-arm64 .
