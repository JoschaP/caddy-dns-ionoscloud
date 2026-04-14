.PHONY: test unit-test lint fmt check build

# Run all tests (requires IONOS_DNS_TOKEN + IONOS_TEST_ZONE)
test:
	go test -v -timeout 120s

# Run unit tests only (no API access needed)
unit-test:
	go test -v -run TestCaddyfile -timeout 30s

# Static analysis
lint:
	go vet ./...

# Format code
fmt:
	gofmt -s -w .

# All checks (lint + format check + unit tests)
check: lint
	@if [ "$$(gofmt -s -l . | wc -l)" -gt 0 ]; then \
		echo "gofmt needed:"; gofmt -s -d .; exit 1; \
	fi
	go test -v -run TestCaddyfile -timeout 30s

# Build custom Caddy with this module
build:
	xcaddy build --with github.com/JoschaP/caddy-dns-ionoscloud=.
