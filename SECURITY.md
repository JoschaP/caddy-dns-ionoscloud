# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |

## Reporting a Vulnerability

If you discover a security vulnerability, please report it responsibly:

1. **Do not** open a public issue
2. Email the maintainer or use [GitHub Security Advisories](https://github.com/JoschaP/caddy-dns-ionoscloud/security/advisories/new)
3. Include steps to reproduce and potential impact

You should receive a response within 48 hours. We will work with you to understand and address the issue before any public disclosure.

## Scope

This module handles API tokens for the IONOS Cloud DNS API. Security concerns include:

- Token exposure in logs or error messages
- Improper token handling in HTTP requests
- Injection vulnerabilities in DNS record names/values
