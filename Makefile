.PHONY: build install clean test lint fmt

# Binary name
BINARY=overcf

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt

# Version info
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS  = -s -w \
	-X github.com/OverseedAI/overcf/internal/cli.version=$(VERSION) \
	-X github.com/OverseedAI/overcf/internal/cli.commit=$(COMMIT) \
	-X github.com/OverseedAI/overcf/internal/cli.date=$(DATE)

# Build directory
BUILD_DIR=./bin

# Main package
MAIN_PKG=./cmd/overcf

# Build the binary
build:
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -ldflags '$(LDFLAGS)' -o $(BUILD_DIR)/$(BINARY) $(MAIN_PKG)

# Install to $GOPATH/bin
install:
	$(GOBUILD) -o $(GOPATH)/bin/$(BINARY) $(MAIN_PKG)

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)

# Run tests
test:
	$(GOTEST) -v ./...

# Run linter
lint:
	golangci-lint run

# Format code
fmt:
	$(GOFMT) -s -w .

# Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Build for multiple platforms
build-all: build-linux build-darwin build-windows

build-linux:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY)-linux-amd64 $(MAIN_PKG)
	GOOS=linux GOARCH=arm64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY)-linux-arm64 $(MAIN_PKG)

build-darwin:
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY)-darwin-amd64 $(MAIN_PKG)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY)-darwin-arm64 $(MAIN_PKG)

build-windows:
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY)-windows-amd64.exe $(MAIN_PKG)
