.PHONY: build clean run test deps

BINARY_NAME=lark
BUILD_DIR=.
export LARK_CONFIG_DIR=.lark

# Version info for local builds
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS = -s -w \
	-X github.com/yjwong/lark-cli/internal/cmd.version=$(VERSION) \
	-X github.com/yjwong/lark-cli/internal/cmd.commit=$(COMMIT) \
	-X github.com/yjwong/lark-cli/internal/cmd.date=$(DATE)

build:
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/lark

clean:
	rm -f $(BUILD_DIR)/$(BINARY_NAME)

run: build
	./$(BINARY_NAME) $(ARGS)

test:
	go test -v ./...

deps:
	go mod tidy
	go mod download

# Install to go bin
install:
	go install ./cmd/lark

# Install to vault tools/bin
install-local:
	go build -ldflags "$(LDFLAGS)" -o ../bin/lark ./cmd/lark
