# IONOS Cloud DNS module for Caddy

[![Go Tests](https://github.com/JoschaP/caddy-dns-ionoscloud/actions/workflows/test.yml/badge.svg)](https://github.com/JoschaP/caddy-dns-ionoscloud/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/JoschaP/caddy-dns-ionoscloud.svg)](https://pkg.go.dev/github.com/JoschaP/caddy-dns-ionoscloud)
[![Go Report Card](https://goreportcard.com/badge/github.com/JoschaP/caddy-dns-ionoscloud)](https://goreportcard.com/report/github.com/JoschaP/caddy-dns-ionoscloud)
[![codecov](https://codecov.io/gh/JoschaP/caddy-dns-ionoscloud/branch/main/graph/badge.svg)](https://codecov.io/gh/JoschaP/caddy-dns-ionoscloud)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A [Caddy](https://caddyserver.com) DNS provider module for the [IONOS Cloud DNS API](https://dns.de-fra.ionos.com/swagger-ui). Enables automatic HTTPS certificates (including **wildcards**) via DNS-01 ACME challenges.

> **Important:** This module uses the **IONOS Cloud DNS API** (`dns.de-fra.ionos.com`), NOT the IONOS Hosting DNS API (`api.hosting.ionos.com`). These are completely different products with different authentication. If you use IONOS shared hosting, see [caddy-dns/ionos](https://github.com/caddy-dns/ionos) instead.

## Features

- Automatic TLS certificates via DNS-01 ACME challenges (including **wildcard** certs)
- Full [libdns](https://github.com/libdns/libdns) implementation: `GetRecords`, `AppendRecords`, `SetRecords`, `DeleteRecords`
- Caddyfile support with block and shorthand syntax
- Environment variable substitution for secure token handling (`{$VAR}`, `{env.VAR}`)
- Zone-ID caching with TTL to minimize API calls
- Configurable API endpoint for custom or regional deployments
- Structured error messages with no token leakage
- Debug logging via Caddy's standard `zap.Logger`

## Prerequisites

- Your domain's **NS records must point to IONOS Cloud nameservers** (`ns-ic.ui-dns.de`, `ns-ic.ui-dns.org`, `ns-ic.ui-dns.biz`, `ns-ic.ui-dns.com`). DNS-01 challenges will fail if the zone is not properly delegated.
- [xcaddy](https://github.com/caddyserver/xcaddy) for building Caddy with plugins (`go install github.com/caddyserver/xcaddy/cmd/xcaddy@latest`)

## Quick Start

### 1. Build Caddy with the plugin

```bash
xcaddy build --with github.com/JoschaP/caddy-dns-ionoscloud
```

To pin a specific version:

```bash
xcaddy build --with github.com/JoschaP/caddy-dns-ionoscloud@v1.0.0
```

Or use the pre-built Docker image:

```bash
docker pull ghcr.io/joschap/caddy-dns-ionoscloud:latest
```

Or build your own:

```dockerfile
FROM caddy:2-builder AS builder
RUN xcaddy build --with github.com/JoschaP/caddy-dns-ionoscloud

FROM caddy:2
COPY --from=builder /usr/bin/caddy /usr/bin/caddy
```

### 2. Get an IONOS Cloud API token

See [Authentication](#authentication) below for a detailed step-by-step guide.

```bash
export IONOS_DNS_TOKEN="your-token-here"
```

### 3. Create a Caddyfile

`{$IONOS_DNS_TOKEN}` is [Caddy's environment variable syntax](https://caddyserver.com/docs/caddyfile/concepts#environment-variables) — it reads the token from the environment at startup.

```caddy
*.example.com, example.com {
    tls {
        dns ionoscloud {$IONOS_DNS_TOKEN}
    }

    @www host www.example.com
    handle @www {
        respond "Hello from www!"
    }

    @api host api.example.com
    handle @api {
        reverse_proxy localhost:8080
    }
}
```

### 4. Run Caddy

```bash
./caddy run
```

That's it. Caddy will automatically:

1. Create a `_acme-challenge` TXT record via the IONOS Cloud DNS API
2. Complete the DNS-01 challenge with Let's Encrypt
3. Obtain a wildcard certificate for `*.example.com`
4. Serve your sites over HTTPS
5. Renew the certificate before it expires

## Usage

### Caddyfile

Block syntax (recommended):

```caddy
*.example.com {
    tls {
        dns ionoscloud {
            api_token {$IONOS_DNS_TOKEN}
        }
    }
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

### Configuration Options

| Option | Required | Description |
|--------|----------|-------------|
| `api_token` | Yes | IONOS Cloud API Bearer token with DNS permissions |
| `api_endpoint` | No | Override the API URL (default: `https://dns.de-fra.ionos.com`) |

### JSON Config

```json
{
  "module": "dns.providers.ionoscloud",
  "api_token": "{env.IONOS_DNS_TOKEN}"
}
```

### Docker Compose

```yaml
services:
  caddy:
    image: ghcr.io/joschap/caddy-dns-ionoscloud:latest
    # Or build your own: build: .
    ports:
      - "80:80"
      - "443:443"
      - "443:443/udp"  # HTTP/3
    environment:
      - IONOS_DNS_TOKEN=${IONOS_DNS_TOKEN}
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile
      - caddy_data:/data
      - caddy_config:/config

volumes:
  caddy_data:
  caddy_config:
```

## Authentication

This module requires an IONOS Cloud API token with access to your DNS zones. For security, create a dedicated sub-user that only has access to the zones it needs.

### Step 1: Create a sub-user

1. Log in to the [IONOS Cloud DCD](https://dcd.ionos.com)
2. Go to **Management > Users & Groups**
3. Click **Create User** — give it a descriptive name (e.g., `caddy-dns`)
4. No special Cloud API privileges are needed — IONOS Cloud DNS is a separate product with its own access control

### Step 2: Share DNS zones with the sub-user

IONOS Cloud DNS manages access via zone sharing, not via Cloud API user privileges. This must be configured through the DCD web interface — there is no API for this.

1. Go to **Menu > DNS** in the DCD
2. Select the zone you want to manage
3. Under **Sharing**, add the sub-user and grant access
4. Repeat for each zone Caddy should manage

### Step 3: Generate an API token

1. Log in to the DCD **as the sub-user** (use a separate browser or incognito session; the sub-user logs in at [dcd.ionos.com](https://dcd.ionos.com) with its own credentials)
2. Go to **Management > Token Management**
3. Click **Generate Token**
4. Copy the token — it is only shown once

> **Note:** IONOS Cloud API tokens have a configurable TTL. For long-running Caddy instances, set a long-lived token or create a reminder to rotate it before expiry. A 401 error during certificate renewal means the token has expired.

## Testing with Let's Encrypt Staging

Before going to production, always test with the Let's Encrypt **staging** environment to avoid rate limits:

```caddy
{
    acme_ca https://acme-staging-v02.api.letsencrypt.org/directory
}

example.com {
    tls {
        dns ionoscloud {$IONOS_DNS_TOKEN}
    }
}
```

Once certificates are issued successfully, remove the `acme_ca` line to use the production Let's Encrypt servers.

## Troubleshooting

| Error | Cause | Fix |
|-------|-------|-----|
| `zone "example.com" not found` | Zone not in your account or sub-user has no access | Share the zone with the sub-user in DCD > DNS > Sharing |
| `401 Unauthorized` | Invalid or expired token | Generate a new token in DCD > Token Management |
| `409 record conflict` | A record with the same name+type already exists | Delete the conflicting record manually in DCD or use `SetRecords` |
| `timed out waiting for record to fully propagate` | DNS propagation delay or zone not properly delegated | Verify zone NS records point to IONOS nameservers (`ns-ic.ui-dns.*`) |

Enable debug logging in Caddy to see DNS API operations:

```caddy
{
    debug
}
```

All log messages from this module are prefixed with `ionoscloud` — filter with `grep ionoscloud` or search for `dns.providers.ionoscloud` in the JSON log output.

Verify your plugin is loaded:

```bash
./caddy list-modules | grep ionoscloud
```

## Updating

To update to a new version, rebuild Caddy with the updated plugin:

```bash
xcaddy build --with github.com/JoschaP/caddy-dns-ionoscloud@v0.2.0
```

For Docker, rebuild your image and redeploy. Caddy stores certificates in its data volume — they persist across rebuilds.

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

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full development guide.

### Prerequisites

- Go 1.25+
- An IONOS Cloud account with DNS zones (for integration tests)

### Run Tests

```bash
# Unit tests only (no API access needed)
go test -v -run TestCaddyfile

# Integration tests (requires IONOS credentials)
IONOS_DNS_TOKEN="$(cat .credentials/ionos_dns_token)" \
IONOS_TEST_ZONE="$(cat .credentials/test_zone)" \
go test -v
```

You can store credentials in a `.credentials/` directory (git-ignored):

```bash
mkdir -p .credentials
echo "your-api-token-here" > .credentials/ionos_dns_token
echo "your-test-zone.example.com" > .credentials/test_zone
chmod 600 .credentials/*
```

> **Warning:** Integration tests create and delete real DNS records. Always use a dedicated test zone.

## Releasing

```bash
git tag v1.0.0
git push origin v1.0.0
```

This triggers the CI pipeline which runs tests and creates a GitHub Release.

## License

MIT — see [LICENSE](LICENSE).

## See Also

- [Caddy](https://caddyserver.com) — the web server
- [libdns](https://github.com/libdns/libdns) — the DNS provider interface
- [IONOS Cloud DNS API](https://dns.de-fra.ionos.com/swagger-ui) — API documentation
- [caddy-dns/ionos](https://github.com/caddy-dns/ionos) — for IONOS **Hosting** DNS (different API)
