# Contributing to caddy-dns-ionoscloud

Thank you for your interest in contributing! This document provides guidelines for contributing to this project.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone git@github.com:YOUR_USERNAME/caddy-dns-ionoscloud.git`
3. Create a branch: `git checkout -b feature/my-change`
4. Make your changes
5. Run checks: `make check`
6. Commit and push
7. Open a Pull Request

## Development Setup

### Prerequisites

- Go 1.25 or later
- An IONOS Cloud account (for integration tests)
- An API token with DNS permissions (for integration tests)

### Running Tests

```bash
# Unit tests only (no API access needed)
go test -v -run TestCaddyfile

# All tests (requires IONOS credentials)
export IONOS_DNS_TOKEN="your-token"
export IONOS_TEST_ZONE="your-test-zone.example.com"
go test -v
```

You can also store credentials in a `.credentials/` directory (git-ignored):

```bash
mkdir -p .credentials
echo "your-api-token-here" > .credentials/ionos_dns_token
echo "your-test-zone.example.com" > .credentials/test_zone
chmod 600 .credentials/*

IONOS_DNS_TOKEN="$(cat .credentials/ionos_dns_token)" \
IONOS_TEST_ZONE="$(cat .credentials/test_zone)" \
go test -v
```

> **Warning:** Integration tests create and delete real DNS records. Always use a dedicated test zone.

### Code Quality

Before submitting a PR, ensure:

```bash
go vet ./...           # Static analysis
gofmt -s -w .          # Format code
go test -v             # All tests pass
```

## Code Style

- Follow standard Go conventions
- Use `gofmt -s` for formatting
- Error messages use `ionoscloud:` prefix for easy grep
- All exported types and functions need doc comments
- Keep functions focused and small
- Integration tests must clean up created records

## Commit Messages

Use clear, descriptive commit messages:

```
Add support for CAA record type

The IONOS Cloud DNS API supports CAA records but the priority
field was not being set correctly. This fix ensures CAA records
are created with the correct format.
```

## Pull Request Process

1. Update README.md if your change affects usage
2. Add tests for new functionality
3. Ensure all CI checks pass
4. One approval required for merge

## Reporting Issues

When reporting bugs, please include:

- Go version (`go version`)
- Caddy version (if applicable)
- Steps to reproduce
- Expected vs actual behavior
- API error messages (redact your token!)

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
