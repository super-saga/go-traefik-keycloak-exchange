# Traefik Keycloak OIDC Exchange Plugin

[![CI](https://github.com/super-saga/go-traefik-keycloak-exchange/actions/workflows/ci.yml/badge.svg)](https://github.com/super-saga/go-traefik-keycloak-exchange/actions/workflows/ci.yml)
[![Release](https://github.com/super-saga/go-traefik-keycloak-exchange/actions/workflows/release.yml/badge.svg)](https://github.com/super-saga/go-traefik-keycloak-exchange/actions/workflows/release.yml)
[![CodeQL](https://github.com/super-saga/go-traefik-keycloak-exchange/actions/workflows/codeql.yml/badge.svg)](https://github.com/super-saga/go-traefik-keycloak-exchange/actions/workflows/codeql.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/super-saga/go-traefik-keycloak-exchange)](https://github.com/super-saga/go-traefik-keycloak-exchange/blob/main/go.mod)
[![License](https://img.shields.io/github/license/super-saga/go-traefik-keycloak-exchange)](https://github.com/super-saga/go-traefik-keycloak-exchange/blob/main/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/super-saga/go-traefik-keycloak-exchange)](https://goreportcard.com/report/github.com/super-saga/go-traefik-keycloak-exchange)
[![Pkg Go Dev](https://pkg.go.dev/badge/github.com/super-saga/go-traefik-keycloak-exchange.svg)](https://pkg.go.dev/github.com/super-saga/go-traefik-keycloak-exchange)
[![Latest Tag](https://img.shields.io/github/v/tag/super-saga/go-traefik-keycloak-exchange?sort=semver)](https://github.com/super-saga/go-traefik-keycloak-exchange/releases)

Provider: PT Saga Tekno Studio

## Personal

- LinkedIn: [Dede Kurniawan](https://www.linkedin.com/in/dedekrnwan/)
- Threads: [Dede Kurniawan](https://www.threads.com/@dedekrnwan_)
- Product: https://saga.co.id

This middleware sits in front of your service. It reads `X-Client-ID` and `X-Client-Secret`, calls Keycloak to get a new access token, then forwards the request to your target service with a new `Authorization: Bearer <token>` header.

Request flow:

1. Client sends a request to Traefik with `X-Client-ID` and `X-Client-Secret`.
2. Middleware calls Keycloak token endpoint using client credentials.
3. Middleware receives a JWT access token.
4. Middleware removes the client headers and forwards the request with `Authorization: Bearer <token>`.
5. Target service receives the new `Authorization` header and does not need to handle Keycloak itself.

## Features

- Exchanges client credentials with Keycloak and injects `Authorization` header with a new access token.
- Default headers: `X-Client-ID` and `X-Client-Secret`
- Header names are configurable, and can be disabled
- Optional or required credential headers
- Debug playground server for local testing

## Configuration

The only mandatory config is the Keycloak realm base URL.

Example Keycloak realm base:

```
https://keycloak.example.com/realms/myrealm
```

The middleware will call:

```
{KEYCLOAK_URL}/protocol/openid-connect/token
```

### Options

| Option | Type | Default | Description |
|---|---|---|---|
| `keycloakURL` | string | required | Keycloak realm base URL |
| `clientIDHeader` | string | `X-Client-ID` | Header for client id, empty disables |
| `clientSecretHeader` | string | `X-Client-Secret` | Header for client secret, empty disables |
| `requireClientCredentials` | bool | `true` | If `false`, missing headers pass through unchanged |
| `tokenRequestTimeoutSeconds` | int | `10` | HTTP client timeout for Keycloak token exchange |

## Using the Plugin

You can use this plugin in two ways:

1. Local plugin mode for development without publishing.
2. Public GitHub module for production (and optionally listed in the Traefik Plugin Catalog).

### Option A: Local Plugin Mode

Place the source code under `./plugins-local/src/github.com/super-saga/go-traefik-keycloak-exchange` and enable `experimental.localPlugins`.

Static config:

```yaml
experimental:
  localPlugins:
    oidc-auth-middleware:
      moduleName: "github.com/super-saga/go-traefik-keycloak-exchange"
```

Dynamic config:

```yaml
http:
  middlewares:
    oidc-auth:
      plugin:
        oidc-auth-middleware:
          keycloakURL: "https://keycloak.example.com/realms/myrealm"
          clientIDHeader: "X-Client-ID"
          clientSecretHeader: "X-Client-Secret"
          requireClientCredentials: true
          tokenRequestTimeoutSeconds: 10

Attach the middleware to a router:

```yaml
http:
  routers:
    api:
      rule: "Host(`api.localhost`)"
      entryPoints:
        - web
      middlewares:
        - oidc-auth
      service: api-service
```
```

### Option B: Public GitHub Module

Push the repository to GitHub and tag a version (for example `v0.1.0`). Traefik will download it by module name and version.

Static config:

```yaml
experimental:
  plugins:
    oidc-auth-middleware:
      moduleName: "github.com/super-saga/go-traefik-keycloak-exchange"
      version: "v0.1.0"
```

Dynamic config:

```yaml
http:
  middlewares:
    oidc-auth:
      plugin:
        oidc-auth-middleware:
          keycloakURL: "https://keycloak.example.com/realms/myrealm"
          clientIDHeader: "X-Client-ID"
          clientSecretHeader: "X-Client-Secret"
          requireClientCredentials: true
          tokenRequestTimeoutSeconds: 10
```

## Marketplace Requirements

To appear in the Traefik Plugin Catalog (Marketplace), the GitHub repository must:

- Be public and not a fork
- Include `.traefik.yml` in the repository root
- Include a valid `go.mod`
- Have the `traefik-plugin` topic set
- Be versioned with a Git tag
- Vendor external dependencies in the repository if you add any

## Versioning

Tag releases with `vX.Y.Z` so Traefik can download the plugin by version.

## Docker Image (Optional)

This repository includes a Dockerfile that bundles the plugin in `/plugins-local` so you can run Traefik without publishing the plugin.

```bash
docker build -t traefik-with-oidc-plugin .
```

## Debug Playground

Run the local playground server:

```bash
KEYCLOAK_URL="https://keycloak.example.com/realms/myrealm" TOKEN_REQUEST_TIMEOUT_SECONDS=10 go run debug/.
```

Send a request with client headers. The middleware will return the forwarded `Authorization` header in the response so you can see the new token:

```bash
curl -H "X-Client-ID: service-a" \
     -H "X-Client-Secret: secret" \
     http://localhost:8080
```

Example response (token returned by Keycloak and forwarded downstream):

```json
{"message":"OK - middleware passed","token":"Bearer eyJhbGciOi..."}
```

## Testing Notes

- The middleware forwards requests only after a successful token exchange.
- When `requireClientCredentials` is `false` and headers are missing, the request passes through without changes.
- The middleware never logs header values or tokens.
