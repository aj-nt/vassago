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

## Development Workflow

1. **Branch from `main`**: `git checkout -b phase-N/your-feature-name`
2. **Push often** — CI runs on every push as a safety net, not a gate
3. **Code-complete** → run QA and Red Team passes locally
4. **Create a PR** against `main` — CI must be green before review
5. **Maintainer approves** → merge to main
6. **Pull to other machines** — verify tests pass

### QA Gates (Required Before Every Merge to `main`)

**1. QA Pass** — does it do what it says?
- All tests pass: `make test-race`
- New code has test coverage
- Edge cases tested (empty input, max limits, concurrent access)
- gRPC returns proper status codes

**2. Red Team QA Pass** — what would break it?
- Crash it: bad payloads, nil fields, oversized input
- Corrupt it: concurrent writes to same key, mid-migration writes
- Starve it: large data, many agents, full hot block budgets
- Cheat it: boundary timestamps, priority overflow, category injection

**If it breaks, add a test, fix the code, re-run both gates.**

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