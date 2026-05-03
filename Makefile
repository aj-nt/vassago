.PHONY: build test test-race lint proto clean generate all deploy setup

# Binary output directory
BINARY_DIR=bin

# Build all binaries
build:
	go build -o $(BINARY_DIR)/mnemosyne-daemon ./daemon/cmd/mnemosyne-daemon
	go build -o $(BINARY_DIR)/mnemosyne-cli ./daemon/cmd/mnemosyne-cli
	go build -o $(BINARY_DIR)/mnemosyne-agent ./agent/cmd/mnemosyne-agent
	go build -o $(BINARY_DIR)/mnemosyne-import ./daemon/cmd/mnemosyne-import
	go build -o $(BINARY_DIR)/mnemosyne ./cmd/mnemosyne

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

# Generate Go code from proto
generate:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		-I proto proto/mnemosyne.proto proto/messaging.proto

# Clean build artifacts
clean:
	rm -rf $(BINARY_DIR)/
	rm -f $(shell find . -name '*.test' -o -name '*.out')

# Tidy dependencies
tidy:
	go mod tidy

# Build + test + lint
all: build test-race lint

# Deploy to local machine (build + copy to ~/bin)
deploy: build
	mkdir -p $(HOME)/bin
	cp $(BINARY_DIR)/mnemosyne-daemon $(HOME)/bin/
	cp $(BINARY_DIR)/mnemosyne-cli $(HOME)/bin/
	cp $(BINARY_DIR)/mnemosyne-agent $(HOME)/bin/
	cp $(BINARY_DIR)/mnemosyne $(HOME)/bin/
	@echo "✓ Binaries deployed to $(HOME)/bin/"

# Deploy to Studio over SSH
deploy-studio: build
	scp $(BINARY_DIR)/mnemosyne-daemon $(BINARY_DIR)/mnemosyne-cli $(BINARY_DIR)/mnemosyne-agent $(BINARY_DIR)/mnemosyne studio:~/bin/
	ssh studio 'codesign --force --sign - ~/bin/mnemosyne-daemon ~/bin/mnemosyne-cli ~/bin/mnemosyne-agent ~/bin/mnemosyne'
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