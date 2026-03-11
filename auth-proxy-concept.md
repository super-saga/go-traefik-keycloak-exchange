# Auth Proxy Concept — Traefik + Keycloak

## Overview

A centralized authentication layer sitting between all incoming requests and backend services. No service handles auth logic individually — everything is enforced at the gateway level.

---

## Architecture

```
Client
  │
  ▼
Traefik (Entrypoint)
  │
  ▼
OIDC Auth Middleware (Plugin or ForwardAuth)
  │
  ├─► [Has X-Client-Id + X-Client-Secret headers?]
  │       │
  │       ├─ YES ──► Exchange with Keycloak (Client Credentials Grant)
  │       │               │
  │       │               ├─ Invalid ──► 401 Unauthorized (block)
  │       │               └─ Valid  ──► Inject Authorization: Bearer <token>
  │       │
  │       └─ NO ───► [Has Authorization: Bearer <token>?]
  │                       │
  │                       ├─ NO  ──► 401 Unauthorized (block)
  │                       └─ YES ──► Introspect/Validate token with Keycloak
  │                                       │
  │                                       ├─ Invalid ──► 401 Unauthorized (block)
  │                                       └─ Valid  ──► Forward request as-is
  ▼
Traefik (Forward to Target Service)
  │
  ▼
Target Service
(receives Authorization: Bearer <token>)
```

---

## Authentication Flows

### Flow 1 — Client Credentials (Service-to-Service)

Used when a client sends raw credentials and needs a token issued.

```
Client ──[X-Client-Id, X-Client-Secret]──► Middleware
Middleware ──[POST /token, grant_type=client_credentials]──► Keycloak
Keycloak ──[access_token]──► Middleware
Middleware ──[Authorization: Bearer <token>]──► Target Service
```

**When to use:** Internal microservices, machine-to-machine calls, CI/CD pipelines hitting internal APIs.

### Flow 2 — Bearer Token Passthrough

Used when a client already has a valid token (previously obtained from Keycloak).

```
Client ──[Authorization: Bearer <token>]──► Middleware
Middleware ──[POST /token/introspect]──► Keycloak
Keycloak ──[{ active: true/false }]──► Middleware
Middleware ──[forward as-is if valid]──► Target Service
```

**When to use:** Frontend apps, mobile clients, any consumer that already authenticates with Keycloak directly.

### Flow 3 — No Credentials (Block)

If neither `X-Client-Id`/`X-Client-Secret` nor `Authorization` header is present, the request is rejected immediately without touching Keycloak.

```
Client ──[no auth headers]──► Middleware ──► 401 Unauthorized
```

---

## Component Responsibilities

| Component | Role |
|---|---|
| **Traefik** | Entrypoint, routing, applies middleware to routes |
| **OIDC Middleware** | Validates/exchanges credentials, injects token headers |
| **Keycloak** | Issues tokens, introspects tokens, manages clients/realms |
| **Target Service** | Receives requests with `Authorization: Bearer` already set — no auth logic needed |

---

## Middleware Options

### Option A — Traefik Plugin (Native, no sidecar)

Runs inside Traefik itself. No extra container.

| Plugin | Client Credentials | Bearer Validation | Notes |
|---|---|---|---|
| `sevensolutions/traefik-oidc-auth` | ❌ | ✅ | Best maintained, clean config |
| `lukaszraczylo/traefikoidc` | ❌ | ✅ | More features (PKCE, RBAC, Redis sessions) |
| `keycloakopenid` | ✅ | ✅ | Supports client credentials flow |
| `go-traefik-keycloak-exchange` | ✅ | ❌ | Focused on header exchange and token injection |

**Limitation:** Most native plugins do not support the optional `X-Client-Id`/`X-Client-Secret` exchange flow out of the box.

### Option B — ForwardAuth Sidecar (Custom)

A small dedicated service (Node.js / Go / Python) that Traefik calls via `forwardAuth`. Full control over both flows.

```
Traefik ──[forwardAuth]──► Auth Sidecar ──► Keycloak
                                │
                         Returns 200 (pass) or 401 (block)
                         Sets Authorization header
```

**Advantage:** Can implement exactly the optional header logic described above — check for `X-Client-Id`/`X-Client-Secret`, fall back to bearer token introspection, block if neither.

### Option C — Gatekeeper (gogatekeeper)

Purpose-built Keycloak proxy. Supports both flows natively. Run in `--no-proxy` mode to act as a ForwardAuth validator.

---

## Header Contract

| Header (Inbound from Client) | Purpose |
|---|---|
| `X-Client-Id` | Optional. Client ID for credential exchange |
| `X-Client-Secret` | Optional. Client secret for credential exchange |
| `Authorization: Bearer <token>` | Optional. Pre-obtained token for passthrough |

| Header (Outbound to Target Service) | Purpose |
|---|---|
| `Authorization: Bearer <token>` | Always present if request is allowed through |
| `X-Auth-User` | Optional. Username from token claims |
| `X-Auth-Email` | Optional. Email from token claims |

---

## Keycloak Setup Requirements

### Realm
- Create a dedicated realm (e.g., `myrealm`)

### Clients Needed

| Client ID | Type | Purpose |
|---|---|---|
| `auth-middleware` | Confidential | Used by middleware to exchange credentials and introspect tokens |
| `service-a`, `service-b`, etc. | Confidential | Represents each service that uses client credentials flow |

### Client Configuration
- Enable **Service Accounts** on `auth-middleware` client
- Grant `token-introspection` scope to `auth-middleware`
- Each service client must have **Service Accounts Enabled**

---

## File Structure

```
.
├── docker-compose.yml
├── traefik/
│   └── dynamic/
│       ├── middlewares.yml     # Auth middleware definition
│       └── routers.yml         # Route + service definitions per target
└── auth-middleware/            # Only needed for Option B (custom sidecar)
    ├── Dockerfile
    ├── package.json
    └── index.js
```

---

## Decision Guide

```
Do you need X-Client-Id / X-Client-Secret exchange?
│
├─ NO  ──► Use traefik-oidc-auth plugin (simplest, no sidecar)
│
└─ YES ──► Do you also need bearer token introspection?
           │
           ├─ NO  ──► Use go-traefik-keycloak-exchange (this project)
           │
           └─ YES ──► Do you want to avoid writing custom code?
                     │
                     ├─ YES ──► Use keycloakopenid plugin or gogatekeeper
                     │
                     └─ NO  ──► Write a custom ForwardAuth sidecar (full control)
```

---

## Security Notes

- Always use HTTPS in production — set `cookie-secure: true` and HTTPS entrypoints in Traefik
- Rotate `clientSecret` and `encryptionKey` regularly
- Scope token introspection to a dedicated client — don't reuse application clients
- Set short token TTLs in Keycloak and enable refresh tokens only where needed
- Never log full `Authorization` header values in middleware
