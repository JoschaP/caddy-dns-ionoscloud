# IONOS Cloud DNS module for Caddy

This package contains a DNS provider module for [Caddy](https://caddyserver.com).
It can be used to manage DNS records with the [IONOS Cloud DNS API](https://dns.de-fra.ionos.com/swagger-ui).

> **Note:** This module uses the **IONOS Cloud DNS API** (`dns.de-fra.ionos.com`),
> NOT the IONOS Hosting DNS API (`api.hosting.ionos.com`). These are different products.

## Usage

### Build

```bash
xcaddy build --with github.com/jprasse/caddy-dns-ionoscloud
```

### Caddyfile

```caddy
*.example.com {
    tls {
        dns ionoscloud {
            api_token {$IONOS_DNS_TOKEN}
        }
    }
}
```

Or inline:

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

## Authentication

Create an IONOS Cloud API token at [DCD](https://dcd.ionos.com) → Management → Token Management.

The token needs **DNS** permissions. For least privilege, create a sub-user with only DNS zone management rights.

Set the token as environment variable:

```bash
export IONOS_DNS_TOKEN="your-token-here"
```

## Supported Record Types

All DNS record types supported by the IONOS Cloud DNS API (A, AAAA, CNAME, MX, TXT, SRV, CAA, etc.).

## License

MIT — see [LICENSE](LICENSE).
