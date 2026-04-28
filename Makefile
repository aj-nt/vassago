.PHONY: build test test-race lint proto clean generate all

# Build the daemon binary
build:
	go build -o bin/mnemosyne-daemon ./daemon/cmd/mnemosyne-daemon
	go build -o bin/mnemosyne-cli ./daemon/cmd/mnemosyne-cli

# Run all tests
test:
	go test ./daemon/...

# Run all tests with race detector
test-race:
	go test -race ./daemon/...

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
		-I proto proto/mnemosyne.proto

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f $(shell find . -name '*.test' -o -name '*.out')

# Tidy dependencies
tidy:
	go mod tidy

# Build + test + lint
all: build test-race lint