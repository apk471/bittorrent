BINARY   := bittorrent
CMD_DIR  := ./cmd/btdemo
BUILD_DIR := build
VERSION  ?= $(shell git describe --tags --dirty --always 2>/dev/null || echo "dev")
LDFLAGS  := -ldflags="-s -w -X main.ldVersion=$(VERSION)"

GOOS     ?= $(shell go env GOOS)
GOARCH   ?= $(shell go env GOARCH)

.PHONY: all build install release clean test lint

all: build

build:
	@echo "Building $(BINARY) for $(GOOS)/$(GOARCH)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) $(CMD_DIR)

install:
	go install $(LDFLAGS) $(CMD_DIR)

release: clean
	@mkdir -p $(BUILD_DIR)
	for os in linux darwin; do \
		for arch in amd64 arm64; do \
			name="$(BINARY)-$${os}-$${arch}"; \
			echo "Building $${name}..."; \
			GOOS=$${os} GOARCH=$${arch} CGO_ENABLED=0 go build $(LDFLAGS) -o "$(BUILD_DIR)/$${name}" $(CMD_DIR); \
		done; \
	done
	cd $(BUILD_DIR) && sha256sum $(BINARY)-* > sha256sums.txt
	@echo "Release builds in $(BUILD_DIR)/"

clean:
	rm -rf $(BUILD_DIR)

test:
	go test ./...

lint:
	go vet ./...
