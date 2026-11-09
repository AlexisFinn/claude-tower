GO_ENV := GOPROXY=off GOFLAGS=-mod=readonly
BIN    := claude-tower
PREFIX := $(HOME)/.local/bin

.PHONY: build test install

build:
	$(GO_ENV) go build -o $(BIN) .

test:
	$(GO_ENV) go vet ./...
	$(GO_ENV) go test ./...

install:
	mkdir -p $(PREFIX)
	$(GO_ENV) go build -o $(PREFIX)/$(BIN) .
