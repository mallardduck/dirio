# DirIO REST API (`/.dirio/api/v1/`)

## Overview

DirIO exposes a small set of HTTP endpoints under `/.dirio/api/v1/` for operations that have no equivalent in the S3 API or the MinIO Admin API. These are the features that are currently only reachable through the web console's internal adapter — ownership management and policy observability — plus a future surface for any other DirIO-specific extensions.

This document defines the API contract, authentication model, and package structure for this layer. It is the normative reference for implementing both the server-side handlers and the `dio` client commands that call them.

---

## Why a Separate API Layer

DirIO has three existing API surfaces:

| Surface | Path prefix | Auth | Purpose |
|---|---|---|---|
| S3 API | `/{bucket}/...` | SigV4 | Object storage operations |
| MinIO Admin API | `/minio/admin/v3/...` | SigV4 + madmin crypto | IAM: users, policies, groups, service accounts |
| Web Console | `/dirio/ui/...` | Session cookie | Browser-only UI |

The web console accesses DirIO-specific features (ownership, policy simulation) through `ConsoleAPI` — an in-process Go interface backed by `internal/console/adapter.go`. This is intentionally a direct service-layer call with no HTTP round-trip. It is not a public API; it is an implementation detail of the console UI.

There is currently no way for an external client (the `dio` binary, scripts, automation) to reach these features over HTTP. The `/.dirio/api/v1/` layer fills that gap:

```
Web Console  →  ConsoleAPI interface  →  adapter.go  →  service layer  (unchanged)
dio client   →  HTTP / SigV4          →  /.dirio/api/v1/*  →  service layer
```

The console and the REST API are **parallel consumers of the same service layer**, not stacked on top of each other. The console does not call the REST API, and the REST API does not call the console.

---

## Design Constraints

- **Always-on** — routes are registered unconditionally in `SetupRoutes`. The `--console` flag and the `noconsole` build tag do not affect this API. `dio` must work even when the console is disabled.
- **Scoped to DirIO-exclusive operations** — this API does not duplicate the S3 API or the MinIO Admin API. IAM (users, policies, groups, service accounts) is handled entirely by `/minio/admin/v3/`.
- **SigV4 authentication** — reuses the existing `auth.AuthMiddleware` so clients only need one credential type. There is no separate API key or session-cookie mechanism.
- **JSON everywhere** — all requests and responses are `application/json`.
- **Versioned prefix** — `/.dirio/api/v1/` allows a clean break in a future `v2` without touching existing client code.
- **Dot-prefix collision safety** — the `/.dirio/` prefix is already established. Because S3 bucket names must start with a letter or digit, no S3 route can ever collide with `/.dirio/` paths.

---

## Authentication & Authorization

All `/.dirio/api/v1/` endpoints require a valid AWS SigV4 signature. Requests without a valid signature receive `401 Unauthorized`.

Beyond authentication, individual endpoints enforce additional authorization checks:

| Endpoint category | Required caller |
|---|---|
| Ownership — read (`GET`) | Any authenticated user |
| Ownership — write (`PUT` transfer) | Admin credentials only |
| Policy simulation | Any authenticated user; `accessKey` in the request body is evaluated as the subject; admin may specify any user |
| Effective permissions | Any authenticated user; callers may query their own access key; admin may query any access key |

"Admin credentials" means the root credentials configured in `.dirio/config.json`, consistent with the existing admin bypass in the policy engine.

---

## Package Structure

Follows the established pattern from `internal/http/server/health/` and `internal/http/server/metrics/`:

```
internal/http/api/dirio/
    ├── routes.go      # RegisterRoutes(r, deps) — wires handlers to router paths
    ├── handler.go     # HTTP handler functions
    └── dirioapi.go    # RouteHandlers interface + Handler struct + error helpers
```

`RegisterRoutes` is called from `server.SetupRoutes` unconditionally, just like `health.RegisterRoutes` and `metrics.RegisterRoutes`. The `RouteDependencies` struct in `server/routes.go` has a `DirioAPI dirioapi.RouteHandlers` field.

The handlers call into the service layer directly — the same service interfaces already used by `adapter.go`. Response types are drawn from `consoleapi/` so that both the console and the REST API share the same wire format for DirIO-specific objects.

---

## Endpoint Reference

### Ownership

#### Get bucket owner

```
GET /.dirio/api/v1/buckets/{bucket}/owner
```

Returns the owner of the named bucket.

**Response `200 OK`:**

```json
{
  "uuid": "3f2504e0-4f89-11d3-9a0c-0305e82c3301",
  "accessKey": "alice",
  "username": "alice"
}
```

For admin-owned buckets (no user owner), `accessKey` and `username` are empty strings:

```json
{
  "uuid": "badfc0de-fadd-fc0f-fee0-000dadbeef00",
  "accessKey": "",
  "username": ""
}
```

Type: `consoleapi.Owner`

**Errors:**

| Status | Condition |
|---|---|
| `401` | Missing or invalid SigV4 signature |
| `404` | Bucket does not exist |

---

#### Transfer bucket ownership

```
PUT /.dirio/api/v1/buckets/{bucket}/owner
```

Transfers ownership of the named bucket to another user. Requires admin credentials.

**Request body:**

```json
{
  "accessKey": "bob"
}
```

**Response `200 OK`:**

```json
{
  "uuid": "...",
  "accessKey": "bob",
  "username": "bob"
}
```

Returns the new owner. Type: `consoleapi.Owner`

**Errors:**

| Status | Condition |
|---|---|
| `400` | Missing or invalid request body |
| `401` | Missing or invalid SigV4 signature |
| `403` | Caller is not admin |
| `404` | Bucket or target user does not exist |

---

#### Get object owner

```
GET /.dirio/api/v1/buckets/{bucket}/objects/{key}
```

`{key}` is the full object key and may contain slashes (URL-encoded as `%2F` or passed raw depending on the client).

Returns the owner of the named object.

**Response `200 OK`:**

```json
{
  "uuid": "3f2504e0-4f89-11d3-9a0c-0305e82c3301",
  "accessKey": "alice",
  "username": "alice"
}
```

Type: `consoleapi.Owner`

**Errors:**

| Status | Condition |
|---|---|
| `401` | Missing or invalid SigV4 signature |
| `404` | Bucket or object does not exist |

---

### Policy Observability

#### Simulate a request

```
POST /.dirio/api/v1/simulate
```

Evaluates whether a given user would be allowed to perform a specific S3 action on a bucket (and optionally a specific object key). Returns the allow/deny decision and the rule that produced it.

**Request body** (`consoleapi.SimulateRequest`):

```json
{
  "accessKey": "alice",
  "bucket":    "my-bucket",
  "action":    "s3:GetObject",
  "key":       "logs/2026/01/report.csv"
}
```

`key` is optional. When omitted, the simulation applies to the bucket root (`*`).

**Response `200 OK`** (`consoleapi.SimulateResult`):

```json
{
  "allowed":     true,
  "reason":      "Bucket policy: statement #2 (explicit Allow, principal *)",
  "matchedRule": "statement #2"
}
```

```json
{
  "allowed":     false,
  "reason":      "Default deny — no matching Allow statement",
  "matchedRule": ""
}
```

**Errors:**

| Status | Condition |
|---|---|
| `400` | Missing required fields (`accessKey`, `bucket`, `action`) |
| `401` | Missing or invalid SigV4 signature |
| `404` | Bucket does not exist |

---

#### Get effective permissions

```
GET /.dirio/api/v1/buckets/{bucket}/permissions/{accessKey}
```

Returns the full set of S3 actions the named user is allowed and denied on the named bucket, evaluated across all applicable policies (bucket policy + attached IAM policies + group policies). Used by `dio simulate --all-actions`.

Non-admin callers may only query their own `accessKey`. Admin callers may query any access key.

**Response `200 OK`** (`consoleapi.EffectivePermissions`):

```json
{
  "accessKey": "alice",
  "bucket": "my-bucket",
  "allowedActions": [
    "s3:GetObject",
    "s3:ListObjectsV2",
    "s3:GetBucketLocation"
  ],
  "deniedActions": [
    "s3:PutObject",
    "s3:DeleteObject"
  ]
}
```

**Errors:**

| Status | Condition |
|---|---|
| `401` | Missing or invalid SigV4 signature |
| `403` | Non-admin caller querying a different user's permissions |
| `404` | Bucket or user does not exist |

---

## Error Response Format

All error responses use a consistent JSON envelope, mirroring the style of the MinIO Admin API error responses for consistency across the `/.dirio/` namespace:

```json
{
  "error": {
    "code":    "NoSuchBucket",
    "message": "The specified bucket does not exist",
    "resource": "/my-bucket"
  }
}
```

`code` is a short camelCase string suitable for programmatic handling. `message` is human-readable. `resource` is optional.

---

## Error Codes

| Code | HTTP Status | Meaning |
|---|---|---|
| `AccessDenied` | `403` | Caller lacks permission for this operation |
| `InvalidRequest` | `400` | Malformed or missing request body |
| `NoSuchBucket` | `404` | Named bucket does not exist |
| `NoSuchObject` | `404` | Named object does not exist |
| `NoSuchUser` | `404` | Named access key does not exist in IAM |
| `InternalError` | `500` | Unexpected server-side error |

---

## Relationship to Other APIs

### S3 API

The S3 API handles all data-plane operations. `/.dirio/api/v1/` does not overlap with any S3 operation. Bucket policy CRUD remains on the S3 API (`GET/PUT/DELETE /{bucket}?policy`) — `/.dirio/api/v1/` does not duplicate it.

### MinIO Admin API (`/minio/admin/v3/`)

All IAM operations — users, policies, groups, service accounts — are handled by the MinIO Admin API. `/.dirio/api/v1/` does not expose IAM CRUD. When `dio iam user list` or `dio sa create` need to run, they call `/minio/admin/v3/`, not this API.

### Web Console

The console continues to use `ConsoleAPI` via `internal/console/adapter.go` for all its operations. The console never calls `/.dirio/api/v1/` — it has no need to incur an HTTP round-trip for data that is available in-process. The two paths to the service layer are independent:

```
Console  →  adapter.go (in-process)  →  service layer
dio      →  /.dirio/api/v1/ (HTTP)   →  service layer
```

If the console is disabled (`--console=false` or build tag `noconsole`), `/.dirio/api/v1/` continues to operate normally.

---

## Versioning Policy

The `v1` prefix is a hard commitment: breaking changes (removed fields, changed semantics, renamed endpoints) require a new `v2` prefix. Additive changes (new optional response fields, new endpoints) may be made in `v1` without a version bump.

`dio` pins to `v1` and will not silently use `v2`. A future `dio` release targeting `v2` will require explicit opt-in (profile config key or flag).

---

## Implementation Phasing

This API is a server-side prerequisite for Phase 7.3 of the DirIO Client. It should be implemented in a dedicated sub-phase before the `dio` client commands that depend on it.

### Phase 7.0 — DirIO API Foundation (server-side)

- `internal/http/api/dirio/` package following `health/` pattern
- `RegisterRoutes` wired into `server.SetupRoutes` unconditionally
- `DirioAPI dirioapi.RouteHandlers` field in `server.RouteDependencies`
- All five endpoints implemented and covered by integration tests in `tests/dirioapi/`
- Error envelope format consistent across all endpoints

### Phase 7.3 — `dio` Client Consumes the API

- `internal/dioclient/dirioapi/` — thin HTTP client wrapper that calls `/.dirio/api/v1/`
- `dio ownership get` / `dio ownership transfer` — call ownership endpoints
- `dio simulate` — calls simulate endpoint; `--all-actions` calls effective-permissions endpoint
