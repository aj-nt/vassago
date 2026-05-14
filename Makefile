.PHONY: build test test-race test-python lint coverage proto clean generate all deploy setup check-licenses install

# Binary output directory
BINARY_DIR=bin

# Build variables
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "v0.0.30-dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILDDATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  = -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildDate=$(BUILDDATE)

# Build all binaries (daemon only)
build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY_DIR)/mnemosyne ./cmd/mnemosyne
	go build -o $(BINARY_DIR)/mnemosyne-daemon ./daemon/cmd/mnemosyne-daemon
	go build -o $(BINARY_DIR)/mnemosyne-cli ./daemon/cmd/mnemosyne-cli
	go build -o $(BINARY_DIR)/mnemosyne-import ./daemon/cmd/mnemosyne-import

# Run all tests
test:
	go test ./...

# Run all tests with race detector
test-race:
	go test -race ./...

# Run integration tests only (requires daemon startup)
test-integration:
	go test -race -timeout 60s -v ./daemon/cmd/mnemosyne-daemon/...

# Lint
lint:
	go vet ./...
	@which golangci-lint >/dev/null 2>&1 && golangci-lint run --timeout=5m ./... || echo "⚠ golangci-lint not installed, skipping"

# Coverage report
coverage:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out
	@echo ""
	@echo "Coverage report: coverage.out"
	@echo "HTML report:     make coverage-html"

coverage-html: coverage
	go tool cover -html=coverage.out -o coverage.html
	@echo "✓ coverage.html written"

# Generate Go code from proto
generate:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		-I proto proto/mnemosyne.proto proto/messaging.proto

# Generate Python gRPC stubs from proto (for SDK)
proto-python:
	pip3 install -q grpcio-tools 2>/dev/null
	python3 -m grpc_tools.protoc \
		-I proto \
		--python_out=proto \
		--grpc_python_out=proto \
		proto/mnemosyne.proto proto/messaging.proto
	@echo "✓ Python stubs generated in proto/"
	@echo "  Copying stubs to SDK..."
	cp proto/mnemosyne_pb2.py sdk/python/src/mnemosyne/_proto/
	cp proto/mnemosyne_pb2_grpc.py sdk/python/src/mnemosyne/_proto/
	cp proto/messaging_pb2.py sdk/python/src/mnemosyne/_proto/
	cp proto/messaging_pb2_grpc.py sdk/python/src/mnemosyne/_proto/
	@echo "✓ SDK stubs updated"

# Test Python SDK
test-python:
	cd sdk/python && PYTHONPATH=src python3 -m pytest tests/ -v

# Clean build artifacts
clean:
	rm -rf $(BINARY_DIR)/
	rm -f $(shell find . -name '*.test' -o -name '*.out')

# Tidy dependencies
tidy:
	go mod tidy

# Build + test + lint
all: check-licenses build test-race lint

# Check license headers on all source files
check-licenses:
	@./scripts/check-license-headers.sh

# Deploy to local machine (build + copy to ~/bin and ~/.local/bin)
deploy: build
	mkdir -p $(HOME)/bin $(HOME)/.local/bin
	cp $(BINARY_DIR)/mnemosyne-daemon $(HOME)/bin/
	cp $(BINARY_DIR)/mnemosyne-cli $(HOME)/bin/
	cp $(BINARY_DIR)/mnemosyne $(HOME)/bin/
	cp $(BINARY_DIR)/mnemosyne $(HOME)/.local/bin/
	codesign -s - -f $(HOME)/bin/mnemosyne $(HOME)/bin/mnemosyne-daemon $(HOME)/bin/mnemosyne-cli 2>/dev/null; true
	@echo "✓ Binaries deployed to $(HOME)/bin/ and $(HOME)/.local/bin/"

# Install: build + deploy + add to PATH hint
install: deploy
	@echo ""
	@echo "✓ Installed. Make sure ~/bin is in your PATH:"
	@echo '  export PATH="$$HOME/bin:$$PATH"'
	@echo ""
	@echo "Then run: mnemosyne init"

# Deploy to Studio over SSH
deploy-studio: build
	scp $(BINARY_DIR)/mnemosyne-daemon $(BINARY_DIR)/mnemosyne-cli $(BINARY_DIR)/mnemosyne studio:~/bin/
	ssh studio 'codesign --force --sign - ~/bin/mnemosyne-daemon ~/bin/mnemosyne-cli ~/bin/mnemosyne'
	@echo "✓ Binaries deployed to Studio"

# First-run setup (creates configs, directories, launchd plists)
setup:
	@./scripts/setup.sh

# Setup for Studio
setup-studio:
	@./scripts/setup.sh --studio

# Count tests
test-count:
	@echo "$$(go test ./... -count=1 -race -v 2>&1 | grep -c '^--- PASS') tests passing"
