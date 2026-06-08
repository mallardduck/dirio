# MinIO Binary Consumption Guide

> How each of the three user-facing MinIO binaries consumes the package ecosystem.
> Useful for understanding which packages matter depending on what you're building
> a replacement for, or what you're trying to understand.

---

## Overview

MinIO produces three user-facing binaries, each built from a separate Go module:

| Binary | Module | What it is |
|--------|--------|-----------|
| `minio` | `github.com/minio/minio` | The S3-compatible object storage server |
| `mc` | `github.com/minio/mc` | Command-line client for S3-compatible servers |
| `console` | `github.com/minio/console` | Web-based admin UI (runs as a backend + embedded assets) |

All three are built with `go build` and produce a single self-contained binary. None
requires a separate runtime or external process at startup.

---

## The `minio` server binary

### What it bundles

The server binary is the most complex consumer in the graph. It directly imports
**all three** Tier 2 shared SDKs, **all three** Tier 1 siblings (server imports console;
console imports mc), and a wide set of Tier 3 primitives.

### The console embed pattern

```
minio/minio
  └─ imports github.com/minio/console
       └─ console's init() registers its HTTP handlers on the router
       └─ console's embedded React assets are served from memory
  └─ the minio/mux router is also imported directly for the S3 API surface
```

The server does not launch a separate console process. When you run `minio server`,
the console web UI is available at port 9001 (by default) because the console package's
`go:embed` filesystem and HTTP handlers are registered in the same binary. This is why
`minio/console` must be a proper Go *library* (importable module) rather than a
standalone CLI program.

### Key package roles inside the server

| Package | Role in the server |
|---------|-------------------|
| `minio/console` | Embedded web UI — registers REST API handlers + serves built React app |
| `minio/minio-go/v7` | Used internally for bucket replication: the server acts as its own S3 client when syncing data to remote sites |
| `minio/madmin-go/v3` | The server *implements* the admin API defined here; it also uses the types for inter-node communication in distributed deployments |
| `minio/pkg/v3` | TLS certificate loading, credential validation, network dialing utilities |
| `minio/kms-go` | Communicates with an external KES/KMS for envelope encryption of objects |
| `minio/mux` | Routes all HTTP requests — S3 API, admin API, and console API all share one `mux.Router` |
| `minio/sio` | Encrypts object data streams at rest using the DARE format |
| `minio/highwayhash` | Computes fast content hashes for integrity checking and erasure coding |
| `minio/simdjson-go` | Parses large JSON documents (bucket notifications, IAM policies) at high throughput |
| `minio/selfupdate` | Enables `mc admin update` to hot-update the server binary |
| `minio/csvparser` | Implements S3 Select for CSV objects |
| `minio/xxml` | Parses and serialises S3 protocol XML (ListObjectsV2, PutBucketPolicy, etc.) |
| `minio/zipindex` | Supports ranged GET inside ZIP objects without reading the full file |
| `minio/dnscache` | Caches DNS lookups for inter-node peer communication in distributed mode |

---

## The `mc` CLI binary

### What it does

`mc` is a Unix-like toolkit for S3-compatible storage. It provides commands analogous
to `ls`, `cp`, `mirror`, `find`, `diff`, plus MinIO-specific admin subcommands under
`mc admin`.

### How it consumes the package graph

`mc` is a lighter consumer than the server. It needs the *client* side of every API
but none of the server-side implementation.

```
mc binary
  ├─ minio/minio-go/v7     — all S3 object operations (ls, cp, rm, presign, …)
  ├─ minio/madmin-go/v3    — all admin operations (mc admin info, mc admin user, …)
  ├─ minio/pkg/v3          — TLS cert loading, credential config (~/.mc/config.json)
  ├─ minio/cli             — argument parsing and command tree
  ├─ minio/selfupdate      — mc update command
  ├─ minio/colorjson       — pretty-prints JSON responses to the terminal
  ├─ minio/filepath        — cross-platform path handling for local↔remote transfers
  └─ mc/pkg/probe          — internal error tracing (stack-enriched errors)
```

`mc` does **not** import `minio/console` (no web UI) or `minio/mux` (no HTTP server).

### The `mc admin` subcommands

All `mc admin` commands — `mc admin info`, `mc admin user add`, `mc admin policy set`,
`mc admin heal`, etc. — are implemented against the `minio/madmin-go` SDK. This SDK
defines typed request/response structs for every admin operation. `mc` calls them over
HTTP to any running `minio` server (or AIStor instance).

---

## The `console` web UI binary

### What it is

`console` is a Go backend that serves a compiled React single-page application and a
generated REST API (using the `go-openapi` code generator toolchain). It can be deployed
as a standalone binary pointing at a `minio` server, but the more common deployment is
*embedded inside the server binary* (see the server section above).

### How it consumes the package graph

```
console binary
  ├─ minio/minio-go/v7     — direct S3 operations (bucket browsing, object uploads)
  ├─ minio/madmin-go/v3    — all admin operations the UI exposes
  ├─ minio/pkg/v3          — TLS config, JWT utilities
  ├─ minio/mc (indirect)   — reuses mc's file-transfer and alias-management logic
  ├─ go-openapi suite       — generated REST server scaffolding from swagger.yml
  └─ minio/highwayhash     — hashing for session tokens
```

### Why console imports mc

`console` imports `minio/mc` as an indirect dependency to reuse mc's internal
file-operation primitives (multipart upload orchestration, progress tracking, transfer
logic). This avoids duplicating complex transfer code between the two tools. The
dependency is one-directional: console → mc, never mc → console.

### Standalone vs. embedded mode

| Mode | How it works |
|------|-------------|
| Standalone | `./console server` runs its own HTTP listener (default :9090), proxying API calls to a `CONSOLE_MINIO_SERVER` endpoint |
| Embedded | `minio/minio` imports the console Go package; console registers its handlers on the shared `mux.Router`; single binary serves both S3 (port 9000) and console (port 9001) |

The embedded mode is the default for the `minio` server binary. Standalone mode is
useful for development, debugging, or deploying the console separately against an
existing MinIO/AIStor cluster.

---

## What each binary needs to build

If you are building a compatible replacement and want to understand the minimum you
need to implement or mock:

### To replace the server

You must implement (or remain compatible with) the APIs consumed by `mc` and `console`:
- The **S3 API** — what `minio/minio-go/v7` calls
- The **admin API** — what `minio/madmin-go/v3` calls
- The **console REST API** — what the embedded console frontend calls

You do not need to use any of the `minio/*` primitives directly; you can substitute
your own encryption, hashing, or routing implementations.

### To replace or extend mc

You only need `minio/minio-go/v7` and `minio/madmin-go/v3` from the minio org.
Everything else is optional tooling.

### To replace the console

You need a server that speaks the `minio/madmin-go/v3` admin API. The console is
effectively a UI layer over that API surface.

---

## Dependency footprint comparison

| Binary | Direct minio/* deps | Transitive minio/* deps | Approx. total deps |
|--------|--------------------|------------------------|--------------------|
| `minio` | ~18 | ~25 | ~239 (from pkg.go.dev) |
| `mc` | ~8 | ~12 | ~144 (from pkg.go.dev cmd package) |
| `console` | ~6 | ~10 | moderate |

The server's 239-import count reflects its role as an integration point: it pulls in
cloud storage SDKs (Azure, GCP, AWS), message queue clients (NATS, Kafka, NSQ, Redis),
database drivers (MySQL, PostgreSQL), observability stacks (Prometheus, OpenTelemetry),
and SFTP/FTP server implementations — all to support notification targets and gateway
modes that the community edition exposed.
