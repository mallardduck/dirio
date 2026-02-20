# DirIO Console Architecture

## Overview

DirIO includes an embedded web admin console that serves two purposes:

1. **Stopgap access** to DirIO-specific hybrid IAM features that `mc` and S3 clients cannot reach (ownership management, effective permission inspection, advanced policy editing)
2. **Foundation** for a full MinIO-style management UI over time

The console is designed to feel like a mono-repo component today while remaining easily extractable into its own repository if that ever becomes desirable.

---

## Design Goals

- **No HTTP round-trips to self** — console handlers call Go interfaces directly, not HTTP endpoints
- **Isolated from core server** — the `console/` package has zero imports from `internal/`
- **Compile-time optional** — can be stripped entirely with a build tag
- **Extractable** — structured so it can become its own module/repo with minimal changes
- **Configurable** — can be disabled at runtime, and optionally served on a separate port

---

## Package Structure

```
dirio/
├── consoleapi/              # Shared contract — the only coupling point
│   └── api.go               # ConsoleAPI interface + request/response types
│
├── console/                 # The console — NOT inside internal/
│   ├── console.go           # Implements http.Handler, wired via ConsoleAPI
│   ├── handlers/            # HTTP handlers (call ConsoleAPI methods only)
│   └── static/              # Embedded frontend assets (Go embed)
│
├── internal/
│   ├── console/
│   │   └── adapter.go       # Implements ConsoleAPI by calling service layer
│   ├── service/             # Core business logic (unchanged)
│   └── ...
│
└── cmd/dirio/
    ├── wire_console.go      # //go:build !noconsole — registers console handler
    └── wire_console_stub.go # //go:build noconsole  — no-op stub
```

### Key Principle: The `consoleapi/` Seam

`consoleapi/` is a tiny package containing only the interface definition and its request/response types — no business logic, no server imports. It is the **only** thing the console package knows about the rest of DirIO.

```
consoleapi/ ←── console/ imports this
consoleapi/ ←── internal/console/adapter.go implements this
```

This means:
- `console/` never imports `internal/`
- `internal/` never imports `console/`
- To extract `console/` to its own repo: move `consoleapi/` to a public module, update import paths — done

---

## The Interface

`consoleapi/api.go` defines the full surface the console can call:

```go
package consoleapi

type API interface {
    // Users
    ListUsers(ctx context.Context) ([]User, error)
    GetUser(ctx context.Context, accessKey string) (*User, error)
    CreateUser(ctx context.Context, req CreateUserRequest) error
    DeleteUser(ctx context.Context, accessKey string) error
    SetUserStatus(ctx context.Context, accessKey string, enabled bool) error

    // Policies
    ListPolicies(ctx context.Context) ([]Policy, error)
    GetPolicy(ctx context.Context, name string) (*Policy, error)
    CreatePolicy(ctx context.Context, req CreatePolicyRequest) error
    DeletePolicy(ctx context.Context, name string) error
    AttachPolicy(ctx context.Context, policyName, accessKey string) error
    DetachPolicy(ctx context.Context, policyName, accessKey string) error

    // Buckets
    ListBuckets(ctx context.Context) ([]Bucket, error)
    GetBucketPolicy(ctx context.Context, bucket string) (*PolicyDocument, error)
    SetBucketPolicy(ctx context.Context, bucket string, doc PolicyDocument) error

    // Ownership (DirIO-specific — not available via mc or S3 clients)
    GetBucketOwner(ctx context.Context, bucket string) (*Owner, error)
    TransferBucketOwnership(ctx context.Context, bucket, newOwnerAccessKey string) error
    GetObjectOwner(ctx context.Context, bucket, key string) (*Owner, error)

    // Policy Observability (DirIO-specific)
    GetEffectivePermissions(ctx context.Context, accessKey, bucket string) (*EffectivePermissions, error)
    SimulateRequest(ctx context.Context, req SimulateRequest) (*SimulateResult, error)
}
```

The adapter in `internal/console/adapter.go` implements this interface by calling the existing service layer.

---

## Build Tags

The console is excluded from the binary using a standard Go build tag:

```bash
go build                    # includes console (default)
go build -tags noconsole    # strips console entirely
```

Two wiring files in `cmd/dirio/` handle this:

**`wire_console.go`** (`//go:build !noconsole`)
- Imports `console/` and `internal/console/adapter`
- Constructs the adapter, passes it to `console.New()`
- Registers the resulting `http.Handler` on the configured address

**`wire_console_stub.go`** (`//go:build noconsole`)
- Contains only a no-op function with the same signature
- Zero imports, zero overhead

---

## Configuration

### Enable/Disable

```yaml
# ~/.dirio/config.yaml or /etc/dirio/config.yaml
console:
  enabled: true          # default: true; set false to disable entirely
```

CLI flag: `--console` / `--no-console`

### Port / Address

The MinIO admin API (`/minio/admin/v3/*`) **must stay on the main port** — `mc` connects there and this cannot change. The console is independent of this constraint.

| Mode | Config | Behavior |
|------|--------|----------|
| Same port (default) | `console.address` unset | Console served at `/_dirio/ui/` on main port |
| Separate port | `console.address: ":9001"` | Console served on its own listener |

```yaml
console:
  enabled: true
  address: ":9001"    # omit to serve on main port
```

CLI flag: `--console-address :9001`

This mirrors how MinIO handled its console (`--console-address`), so the mental model is familiar to operators coming from MinIO.

### Authentication

The console authenticates using the same admin credentials stored in `.dirio/config.json`. No separate service account is required — the console is an in-process component with direct service access, not an external client.

---

## What the Console Is NOT

- **Not a separate process** — always runs inside the DirIO server binary
- **Not an HTTP client to itself** — no round-trips; direct Go interface calls
- **Not required** — compile or disable it without affecting S3 or admin API behavior
- **Not a replacement for `mc`** — operators continue using `mc` for compatible operations; the console fills gaps `mc` cannot reach

---

## Stopgap Priorities

These are the DirIO-specific features that `mc` and S3 clients cannot provide, listed in order of priority:

### 1. Ownership Management
- View bucket owner (UUID + username display)
- View object owner
- Transfer bucket ownership to another user
- `mc` has no ownership concept; this is only accessible via the console or future DirIO-native client

### 2. Policy Observability
- Show a user's **effective permissions** (bucket policies + IAM policies evaluated together)
- Simulate a request: given user + bucket + action, show allow/deny and which policy rule decided it
- Explain why access was granted/denied (the evaluation trace)
- `mc admin user info` only shows attached policy names, not effective access

### 3. Full S3 Bucket Policy Editor
- View and edit bucket policies as JSON
- `mc anonymous` / `mc policy set` only handle simplified canned policies; the console handles full S3 JSON with conditions and variables

### 4. mc Compatibility Shim Visibility
- Show when a simplified `mc policy set` command was received and how it was translated to a full S3 policy document

---

## Future Expansion (Full MinIO-Style UI)

Once stopgaps are in place, the console can expand to full management UI:

- User/policy CRUD (visual complement to `mc admin`)
- Service account management
- Group management
- Dashboard: server health, storage usage, request metrics
- Audit log viewer (when audit logging is implemented)
- File browser and object upload UI (Phase 8)

---

## Extractability Path

If the console ever needs to become its own repo:

1. Move `consoleapi/` to a public Go module (e.g., `github.com/danvolchek/dirio-consoleapi`)
2. Update `console/` imports to point to the new module — `console/` becomes its own module
3. Update `internal/console/adapter.go` imports — the server side now imports the shared module too
4. Remove `console/` from this repo, add it as a `go.mod` dependency

The interface is the seam. Everything else is just updating import paths.

---

## Relationship to Other IAM Components

```
mc / S3 clients
    │
    ├── S3 API (port 9000)         ← bucket policies, data plane
    ├── MinIO Admin API (port 9000) ← mc admin user/policy commands
    │
DirIO Console
    │
    └── consoleapi.API (Go interface)
            │
            └── internal/console/adapter
                    │
                    └── service layer (same layer as API handlers)
```

The console sits alongside the HTTP API handlers, not above or below them — both are consumers of the same service layer.