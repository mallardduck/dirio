# MinIO Go Package Status Index

> **As of June 2026.** Sources: pkg.go.dev latest-published dates and GitHub repository pages.
> A pkg.go.dev date reflects the last *tagged* release indexed by the Go module proxy — the
> strongest signal for whether a repo is still actively publishing. A repo may receive commits
> to an untagged or private branch without this changing.

---

## Status legend

| Badge | Meaning |
|-------|---------|
| ✅ Active | Public repo, recent tagged releases, accepting contributions |
| ⬛ Archived | GitHub read-only archive; no new commits or PRs accepted |
| ⚠ Gutted | Repo public but core functionality stripped for commercial product |
| ❄ Frozen | Public but no recent tags; feature-complete, not abandoned |

---

## Tier 1 — Application binaries

These are the user-facing programs. Each has its own `go.mod` and release cycle.

### `github.com/minio/minio` — S3 object storage server

| Field | Value |
|-------|-------|
| pkg.go.dev latest | Feb 12, 2026 (pseudo-version) |
| Latest version | `v0.0.0-20260212201848-7aac2a2c5b7c` |
| License | AGPL-3.0 |
| GitHub status | **⬛ Archived Apr 25, 2026 (read-only)** |

The community edition was declared "no longer maintained" on Feb 12 2026 and the repository
was archived (permanently read-only) on Apr 25 2026. Last functional tagged release was
`RELEASE.2025-10-15T17-29-55Z`. The code remains public under AGPLv3 and is forkable.
MinIO Inc. directs users to their commercial **AIStor** product.

Timeline of decline:
- **Apr 2021** — Relicensed from Apache 2.0 to AGPL-3.0
- **May 2025** — Admin console stripped from community edition
- **Oct 2025** — Last security-patch release
- **Dec 2025** — "Maintenance mode" declared in README
- **Feb 2026** — "No longer maintained" commit; first archive
- **Apr 2026** — Permanently re-archived

---

### `github.com/minio/mc` — CLI client

| Field | Value |
|-------|-------|
| pkg.go.dev latest | Nov 6, 2025 |
| Latest version | `v0.0.0` (uses pseudo-versions; binary releases tagged separately) |
| License | AGPL-3.0 |
| GitHub status | **✅ Active** |

Not archived. Issues were filed as recently as May 2026. Binary releases are tagged with
date-based names (`RELEASE.2025-08-13T08-35-41Z`) rather than semver. Receives ongoing
maintenance, likely to support AIStor users who still use `mc` as their CLI tool.

---

### `github.com/minio/console` — Web admin UI

| Field | Value |
|-------|-------|
| pkg.go.dev latest | Feb 11, 2025 |
| Latest version | `v1.7.7-0.20250905210349-2017f33b26e1` (pseudo) |
| License | AGPL-3.0 |
| GitHub status | **⚠ Public — functionally gutted for community use** |

Not formally archived, but in May 2025 the full console was removed from the community
server and replaced with a bare object browser. The `minio/minio` go.mod pins a
*September 2025* pseudo-version commit, indicating active internal development that is
not being tagged publicly — consistent with the full console having moved to a
private/commercial branch under AIStor.

---

## Tier 2 — Shared SDKs

These libraries are imported by all three Tier 1 applications and are maintained
independently of the server's end-of-life.

### `github.com/minio/minio-go/v7` — S3 client SDK

| Field | Value |
|-------|-------|
| pkg.go.dev latest | **Apr 26, 2026** |
| Latest version | `v7.1.0` |
| License | **Apache-2.0** |
| GitHub status | **✅ Actively maintained** |

The healthiest package in the entire graph. Published as recently as April 2026, with
3,000+ downstream importers. Apache-licensed — no AGPL network-service obligations.
Used both internally by the MinIO server (for replication and self-calls) and by any
third-party building against an S3-compatible API. Safe to depend on.

---

### `github.com/minio/madmin-go/v3` — Admin API client SDK

| Field | Value |
|-------|-------|
| pkg.go.dev latest | ~Jun 3, 2026 |
| Latest version | `v3.0.109+` (v4 branch in development) |
| License | AGPL-3.0 |
| GitHub status | **✅ Actively maintained** |

Updated Jun 3 2026 per the GitHub org page. A v4 branch is in development. Contains a
`CLAUDE.md` file indicating AI-assisted maintenance. This library defines the admin API
types shared between the server (which *implements* the API) and mc/console (which *call*
it). It is actively maintained to keep up with AIStor's evolving admin surface.

---

### `github.com/minio/pkg/v3` — Shared utilities

| Field | Value |
|-------|-------|
| pkg.go.dev latest | ~Jun 3, 2026 |
| Latest version | `v3.1.3` (Debian mirrors show `v3.4.2` from Feb 2026) |
| License | AGPL-3.0 |
| GitHub status | **✅ Actively maintained** |

Updated Jun 3 2026. Debian mirrors show a `v3.4.2` tarball dated Feb 17 2026 — active
tagging *after* the server was archived. Contains TLS certificate utilities, credential
helpers, and cross-cutting network utilities shared by all three Tier 1 apps.

---

### `github.com/minio/kms-go` — KMS/KES client SDK

| Field | Value |
|-------|-------|
| pkg.go.dev latest | Feb 25, 2025 (pseudo-version) |
| Latest version | `kes@v0.3.1` / `kms@v0.5.1-pre` |
| License | AGPL-3.0 |
| GitHub status | **✅ Public** |

The *client* SDK for Key Encryption Server (KES) / Key Management Service (KMS).
Distinct from the now-deprecated `minio/kes` *server* repo. The go.mod in
`minio/minio` pins a Feb 2025 pseudo-version, suggesting ongoing but untagged work.

---

### `github.com/minio/kes` — Key Encryption Server (deprecated)

| Field | Value |
|-------|-------|
| Latest version | ~v0.24.x |
| License | AGPL-3.0 |
| GitHub status | **⬛ Archived — explicitly deprecated** |

The GitHub org page labels this "[Deprecated] Key Encryption Server". The KES *server*
functionality has moved into AIStor's commercial product. Superseded by `minio/kms-go`
for the client-side interface.

---

## Tier 3 — Primitives

Low-level libraries with zero `minio/*` dependencies. Each does one specific thing.

| Package | pkg.go.dev latest | Version | License | Status | Notes |
|---------|-------------------|---------|---------|--------|-------|
| `minio/highwayhash` | Oct 30, 2025 | v1.0.3 | Apache-2.0 | ✅ Active | 382 importers; algorithm stable by design |
| `minio/sio` | Nov 24, 2025 | v0.4.1 | Apache-2.0 | ✅ Active | DARE stream encryption; 137 importers |
| `minio/zipindex` | Jan 7, 2026 | v0.4.0 | Apache-2.0 | ✅ Active | Tagged post-server-archive |
| `minio/crc64nvme` | ~2024 | v1.0.1 | Apache-2.0 | ✅ Active | Used by minio-go; indirectly monitored |
| `minio/md5-simd` | ~2024 | v1.1.2 | Apache-2.0 | ❄ Frozen | Algorithm fixed; used by minio-go |
| `minio/simdjson-go` | Mar 11, 2023 | v0.4.5 | Apache-2.0 | ❄ Frozen | No tag in 2+ years; public, not archived |
| `minio/mux` | ~Jul 2024 | v1.9.2 | Apache-2.0 | ❄ Low activity | HTTP router fork; feature-complete |
| `minio/selfupdate` | ~late 2024 | v0.6.0 | Apache-2.0 | ❄ Low activity | Binary updater; future uncertain post-archive |
| `minio/cli` | ~2024 | v1.24.2 | Apache-2.0 | ❄ Low activity | CLI framework; used by mc (still active) |
| `minio/dperf` | ~2024 | v0.6.3 | AGPL-3.0 | ✅ Active | Drive performance tool; own CLI |
| `minio/dnscache` | ~2023 | v0.1.1 | Apache-2.0 | ❄ Frozen | DNS caching; stable, rarely changes |
| `minio/xxml` | ~2023 | v0.0.3 | Apache-2.0 | ❄ Frozen | XML for S3 responses; S3 XML is stable |
| `minio/csvparser` | ~2021 | v1.0.0 | Apache-2.0 | ❄ Dormant | S3 Select CSV; pinned at v1.0.0 for years |

---

## Archived / deprecated (notable)

| Repo | Status | Notes |
|------|--------|-------|
| `minio/operator` | ⬛ Archived | Kubernetes operator; archived alongside the server |
| `minio/kes` | ⬛ Archived | KES server; see `minio/kms-go` for client SDK |

---

## Key observations

**The server archive does not kill the SDK layer.** `minio-go`, `madmin-go`, and `pkg`
are all receiving active commits in 2026. This makes sense: MinIO Inc. needs these SDKs
functional to support AIStor customers using `mc`, the console, and third-party
integrations. The SDKs are the interface; the server is just one implementation of it.

**Watch the license.** `minio-go` is Apache-2.0 (safe to use in proprietary services).
`madmin-go`, `pkg`, `kms-go`, and `dperf` are AGPL-3.0 — any network service that
incorporates them must open-source its modifications. This is relevant if you are
building a commercial replacement.

**"Frozen" ≠ broken.** A frozen package at a stable version (`sio`, `highwayhash`,
`csvparser`) is often the correct end-state for a library that implements something
that doesn't change. These are fine to depend on unless you need new features or
security patches that haven't arrived.

**Private activity may underlie public silence.** The `minio/console` pseudo-version
pattern (internal commits not appearing as public tags) is a signal that activity
continues behind a visibility boundary. Treat any package with recent pseudo-versions
in a live repo's go.mod as potentially active, even if pkg.go.dev shows an old date.
