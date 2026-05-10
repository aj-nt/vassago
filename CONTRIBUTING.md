# Contributing to Mnemosyne

Thank you for your interest in contributing to Mnemosyne! This document outlines the process and guidelines for contributing to the project.

## Quick Start

```bash
git clone https://github.com/aj-nt/mnemosyne.git
cd mnemosyne
make install              # build + install to ~/bin
make test-race            # run all tests with race detector
```

See [README.md](README.md) for full setup instructions.

### Local Development Setup

```bash
# Prerequisites
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Regenerate proto after changing mnemosyne.proto
export PATH="$PATH:$(go env GOPATH)/bin"
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       -I proto proto/mnemosyne.proto
```

### Useful Commands

```bash
make test-race              # all tests with race detector
go test -race ./agent/...   # single package
go vet ./...                # static analysis
./scripts/check-license-headers.sh  # verify license headers
go build ./...              # verify compilation
```

## Development Workflow

1. **Create a branch from `main`**: `git checkout -b feat/short-name` or `fix/short-name`
   - NEVER work directly on `main` for any code change
2. **TDD (Test-Driven Development) for all production code**:
   - Write the test FIRST, verify it FAILS (red)
   - Write the minimum implementation to pass (green)
   - Refactor with tests as safety net (refactor)
   - At the end of each phase, run all three QA passes below
3. **Push often** — CI runs on every push as a safety net, not a gate
4. **Code-complete** → run QA and Red Team passes locally
5. **Create a PR** against `main` — CI must be green before review
6. **Maintainer approves** → merge to main
7. **Pull to other machines** — verify tests pass

### Check-In Procedure (Required Before Every Merge to `main`)

**1. White-Box QA Pass** — code review of every new and modified file
- Trace all code paths, edge cases (nil, empty, max values)
- Verify error handling, concurrency safety, resource cleanup
- Check that all specification requirements are met
- Run: `make test-race`, `go vet ./...`, `./scripts/check-license-headers.sh`

**2. Black-Box QA Pass** — trace every user flow end-to-end
- Walk through each feature as a user would experience it
- Verify error messages are clear and actionable
- Check behavior with no config, empty state, missing dependencies

**3. Red Team QA Pass** — attempt to break every new surface
- Crash it: nil fields, bad payloads, oversized input, invalid types
- Corrupt it: concurrent writes, mid-operation cancellation, partial updates
- Starve it: large data sets, many concurrent agents, full budgets
- Inject it: SQL injection, null bytes, special characters, path traversal
- Spoof it: wrong agent IDs, boundary timestamps, priority overflow

**If anything breaks: add a test, fix the code, re-run all three gates from the beginning.**

### Refactoring Pass (After QA + Red Team)

At the end of every phase, we apply Fowler's catalog transformations:

1. QA Pass → Red Team Pass → both green
2. Read code with fresh eyes, identify smells
3. Pick 5-8 highest-impact refactorings
4. Apply one at a time, run `make test-race` after each
5. Commit: `refactor: <Fowler name> — <what and why>`

## Commit Messages

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add cron job scheduling tool
fix: prevent hot block stale cache race condition
docs: update README test badge count
refactor: Extract Function — split pipeline loop from dispatch
test: add concurrent write Red Team tests for store
chore: update Go version to 1.26
```

## Code Style

- **`gofmt` is law** — run `gofmt -s -w .` before committing
- **Standard Go conventions** — [Effective Go](https://go.dev/doc/effective_go)
- **Error types** in `errors/`, shared helpers in `internal/`
- **Short functions** — if a function is longer than ~40 lines, extract it
- **Named returns** for public functions with multiple return values
- **Table-driven tests** — prefer `[]struct{want, got}` over copy-pasted test functions

## Architecture

```
mnemosyne/
├── cmd/                   # CLI entry points (unified CLI + legacy binaries)
│   ├── mnemosyne/         # Unified CLI (init, start, stop, chat, status, memory, config, version)
│   ├── mnemosyne-daemon/  # Daemon binary
│   └── mnemosyne-agent/   # Agent binary
├── daemon/
│   ├── internal/           # Private packages (store, grpc, pubsub, cron, messaging, scheduler)
│   └── cmd/                # Daemon main
├── agent/
│   ├── pipeline/           # Request processing stages
│   ├── provider/           # LLM providers (OpenAI, Anthropic)
│   ├── tools/              # Agent tools (memory, file, terminal, browser, etc.)
│   └── scheduler/          # Periodic task runner
├── client/                 # Public gRPC client (Apache 2.0)
├── proto/                  # Protocol definitions (Apache 2.0)
└── messaging/              # Platform adapters (Discord, Telegram, iMessage, email)
```

Key principles:
- **`client/` and `proto/`** are Apache 2.0 — keep them independent of AGPL code
- **`daemon/internal/`** is private — never import from outside the daemon
- **Interfaces over structs** — every external dependency has a mock interface
- **Test externally** — integration tests use `bufconn` or real gRPC, never mock private internals

## Testing

```bash
make test-race          # all tests with race detector (required before PR)
go test -race ./...     # equivalent
go vet ./...             # static analysis
make all                 # build + test + lint
```

We have **1230+ tests** with `-race`. All must pass before merging.

### Writing Tests

- **Unit tests** for every new function/method
- **Integration tests** for gRPC endpoints (use `bufconn` listener)
- **Red Team tests** when you add gRPC endpoints or store operations (try to break it)
- **Table-driven** wherever possible

## Reporting Bugs

Open a [GitHub issue](https://github.com/aj-nt/mnemosyne/issues/new?template=bug_report.yml) with:

1. Steps to reproduce
2. Expected vs actual behavior
3. `mnemosyne version` output
4. OS and config details

## Suggesting Features

Open a [GitHub Discussion](https://github.com/aj-nt/mnemosyne/discussions) first. We prefer small, focused PRs over large rewrites. Check [ROADMAP.md](ROADMAP.md) for planned work.

## License

By contributing, you agree that your contributions will be licensed under the same dual-license model:
- Core code: GNU AGPLv3
- Protocol/Client SDK: Apache 2.0