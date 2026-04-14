# IONOS Cloud DNS module for Caddy

[![Go Tests](https://github.com/JoschaP/caddy-dns-ionoscloud/actions/workflows/test.yml/badge.svg)](https://github.com/JoschaP/caddy-dns-ionoscloud/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/JoschaP/caddy-dns-ionoscloud.svg)](https://pkg.go.dev/github.com/JoschaP/caddy-dns-ionoscloud)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

This package contains a DNS provider module for [Caddy](https://caddyserver.com). It can be used to manage DNS records with the [IONOS Cloud DNS API](https://dns.de-fra.ionos.com/swagger-ui) for automatic HTTPS certificate provisioning via DNS-01 ACME challenges.

> **Important:** This module uses the **IONOS Cloud DNS API** (`dns.de-fra.ionos.com`), NOT the IONOS Hosting DNS API (`api.hosting.ionos.com`). These are completely different products with different authentication. If you use IONOS shared hosting, this module is not for you — see [caddy-dns/ionos](https://github.com/caddy-dns/ionos) instead.

## Features

- DNS-01 ACME challenges for automatic TLS certificates (including **wildcard** certs)
- Full DNS record management (CRUD) via the [libdns](https://github.com/libdns/libdns) interfaces
- Caddy module with Caddyfile support
- Environment variable substitution for secure token handling

## Installation

### Build with xcaddy

```bash
xcaddy build --with github.com/JoschaP/caddy-dns-ionoscloud
```

### Docker

```dockerfile
FROM caddy:2-builder AS builder
RUN xcaddy build --with github.com/JoschaP/caddy-dns-ionoscloud

FROM caddy:2
COPY --from=builder /usr/bin/caddy /usr/bin/caddy
```

## Configuration

### Authentication

Create an IONOS Cloud API token at [DCD](https://dcd.ionos.com) → Management → Token Management.

The token needs **DNS zone management** permissions. For least privilege, create a sub-user with only DNS rights and generate a token for that user.

Set the token as environment variable:

```bash
export IONOS_DNS_TOKEN="your-token-here"
```

### Caddyfile

Wildcard certificate with environment variable:

```caddy
*.example.com {
    tls {
        dns ionoscloud {
            api_token {$IONOS_DNS_TOKEN}
        }
    }
    # your site config...
}
```

Shorthand syntax:

```caddy
*.example.com {
    tls {
        dns ionoscloud {$IONOS_DNS_TOKEN}
    }
}
```

### JSON Config

```json
{
  "module": "dns.providers.ionoscloud",
  "api_token": "{env.IONOS_DNS_TOKEN}"
}
```

## Supported Record Types

All DNS record types supported by the IONOS Cloud DNS API: A, AAAA, CNAME, MX, TXT, SRV, CAA, NS, etc.

## API Compatibility

This module targets the IONOS Cloud DNS API at `dns.de-fra.ionos.com`. It uses:

- `GET /zones` — list zones
- `GET /zones/{zoneId}/records` — list records
- `POST /zones/{zoneId}/records` — create record
- `PUT /zones/{zoneId}/records/{recordId}` — update record
- `DELETE /zones/{zoneId}/records/{recordId}` — delete record

Authentication is via Bearer token in the `Authorization` header.

## Development

### Prerequisites

- Go 1.22+
- An IONOS Cloud account with DNS zones
- An API token with DNS permissions

### Run Tests

Integration tests run against the real IONOS Cloud DNS API. Use a dedicated test zone to avoid affecting production DNS.

```bash
export IONOS_DNS_TOKEN="your-token"
export IONOS_TEST_ZONE="your-test-zone.example.com"
go test -v
```

**Warning:** Tests create and delete real DNS records. Always use a dedicated test zone, never your production zone.

### Run Unit Tests Only

The Caddyfile parsing tests do not require API access:

```bash
go test -v -run TestCaddyfile
```

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-change`)
3. Write tests for your changes
4. Ensure all tests pass (`go test -v`)
5. Run `go vet ./...` and `gofmt -s -w .`
6. Commit with a clear message
7. Open a Pull Request

### Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Error messages start with `ionoscloud:` prefix
- All exported functions need doc comments
- Integration tests must clean up after themselves (delete created records)

## Releasing

Releases are created via GitHub tags:

```bash
git tag v0.1.0
git push origin v0.1.0
```

This triggers the CI pipeline which runs tests and creates a GitHub Release.

## License

MIT — see [LICENSE](LICENSE).

## See Also

- [Caddy](https://caddyserver.com) — the web server
- [libdns](https://github.com/libdns/libdns) — the DNS provider interface
- [IONOS Cloud DNS API](https://dns.de-fra.ionos.com/swagger-ui) — API documentation
- [caddy-dns/ionos](https://github.com/caddy-dns/ionos) — for IONOS **Hosting** DNS (different API)
