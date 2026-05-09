# Pull Request

## Description

<!-- What does this PR change? Why? Link relevant issues. -->

Closes #

## Change Type

- [ ] Bugfix
- [ ] Feature
- [ ] Refactor
- [ ] Documentation
- [ ] Test
- [ ] Infrastructure / CI

## QA Pass

- [ ] `make test-race` passes (all 1230+ tests, race detector clean)
- [ ] New code has test coverage
- [ ] Edge cases tested (empty input, max limits, concurrent access)
- [ ] No panics on malformed input

## Red Team Pass

- [ ] Bad payloads don't crash — malformed gRPC, nil fields, oversized input
- [ ] Concurrent writes don't deadlock or corrupt data
- [ ] Boundary values handled correctly (priority overflow, timestamp gaming)
- [ ] If it breaks, I've added a test for it

## Checklist

- [ ] `go vet ./...` passes
- [ ] `gofmt -s -w .` produces no changes
- [ ] Commits follow [Conventional Commits](CONTRIBUTING.md): `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`
- [ ] No unintended changes to proto files (or proto changes are intentional and regenerated)