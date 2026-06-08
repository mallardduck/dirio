# MinIO Dependency Diagrams

> Mermaid-compatible diagrams. Render with any Mermaid viewer, GitHub markdown preview,
> or paste into https://mermaid.live

---

## Diagram 1 — Full ecosystem overview (three tiers)

Shows the three-tier architecture: application binaries → shared SDKs → primitives.
Arrows represent "imports / depends on."

```mermaid
graph TD
    subgraph T1["Tier 1 — Application binaries"]
        SERVER["minio/minio\nS3 server"]
        MC["minio/mc\nCLI client"]
        CONSOLE["minio/console\nWeb admin UI"]
    end

    subgraph T2["Tier 2 — Shared SDKs"]
        MINIOGO["minio/minio-go/v7\nS3 client SDK"]
        MADMIN["minio/madmin-go/v3\nAdmin API SDK"]
        PKG["minio/pkg/v3\nShared utilities"]
        KMSGO["minio/kms-go\nKMS/KES client"]
    end

    subgraph T3["Tier 3 — Primitives"]
        HWH["minio/highwayhash"]
        SIO["minio/sio"]
        MUX["minio/mux"]
        SIMD["minio/simdjson-go"]
        ZIP["minio/zipindex"]
        CRC["minio/crc64nvme"]
        MD5["minio/md5-simd"]
        CLI["minio/cli"]
        SELF["minio/selfupdate"]
        XXML["minio/xxml"]
    end

    SERVER --> CONSOLE
    SERVER --> MINIOGO
    SERVER --> MADMIN
    SERVER --> PKG
    SERVER --> KMSGO

    MC --> MINIOGO
    MC --> MADMIN
    MC --> PKG

    CONSOLE --> MINIOGO
    CONSOLE --> MADMIN
    CONSOLE --> PKG
    CONSOLE --> MC

    MINIOGO --> MD5
    MINIOGO --> CRC

    SERVER --> HWH
    SERVER --> SIO
    SERVER --> MUX
    SERVER --> SIMD
    SERVER --> ZIP
    SERVER --> XXML
    SERVER --> CLI
    SERVER --> SELF

    MC --> CLI
    MC --> SELF
```

---

## Diagram 2 — Server import detail

What `minio/minio` imports from the minio org, annotated by purpose.

```mermaid
graph LR
    SERVER["minio/minio"]

    subgraph EMBED["Embedded at startup"]
        CONSOLE["minio/console\n(bundles WebUI + REST API)"]
    end

    subgraph PROTO["Protocol & admin"]
        MINIOGO["minio/minio-go/v7\n(S3 — replication, self-calls)"]
        MADMIN["minio/madmin-go/v3\n(admin API types + client)"]
        KMSGO["minio/kms-go\n(encryption key management)"]
    end

    subgraph UTIL["Utilities"]
        PKG["minio/pkg/v3\n(certs, creds, TLS config)"]
        MUX["minio/mux\n(HTTP router)"]
        CLI["minio/cli\n(CLI framework)"]
        SELF["minio/selfupdate\n(binary updates)"]
    end

    subgraph PERF["Performance primitives"]
        HWH["minio/highwayhash\n(fast hashing)"]
        SIO["minio/sio\n(stream encryption)"]
        SIMD["minio/simdjson-go\n(SIMD JSON parsing)"]
        MD5["minio/md5-simd\n(SIMD checksums)"]
        CRC["minio/crc64nvme\n(NVMe CRC64)"]
    end

    subgraph FORMAT["Format parsers"]
        ZIP["minio/zipindex\n(ZIP index)"]
        XXML["minio/xxml\n(S3 XML)"]
        CSV["minio/csvparser\n(S3 Select CSV)"]
        DNS["minio/dnscache\n(DNS cache)"]
    end

    SERVER --> CONSOLE
    SERVER --> MINIOGO
    SERVER --> MADMIN
    SERVER --> KMSGO
    SERVER --> PKG
    SERVER --> MUX
    SERVER --> CLI
    SERVER --> SELF
    SERVER --> HWH
    SERVER --> SIO
    SERVER --> SIMD
    SERVER --> MD5
    SERVER --> CRC
    SERVER --> ZIP
    SERVER --> XXML
    SERVER --> CSV
    SERVER --> DNS
```

---

## Diagram 3 — Shared SDK cross-imports

Shows exactly which packages `minio/console` and `minio/mc` share vs. what is unique to each.

```mermaid
graph TD
    subgraph APPS["Application binaries"]
        CONSOLE["minio/console"]
        MC["minio/mc"]
    end

    subgraph SHARED["Shared by both"]
        MINIOGO["minio/minio-go/v7"]
        MADMIN["minio/madmin-go/v3"]
        PKG["minio/pkg/v3"]
    end

    subgraph CONSOLE_ONLY["Console-only extras"]
        OPENAPI["go-openapi suite\n(generated REST server)"]
        MC_DEP["minio/mc\n(imported for file-ops)"]
    end

    subgraph MC_ONLY["mc-only extras"]
        COLORJSON["minio/colorjson"]
        FILEPATH["minio/filepath"]
        SELF["minio/selfupdate"]
        PROBE["mc/pkg/probe\n(error tracing)"]
        CLI["minio/cli"]
    end

    CONSOLE --> MINIOGO
    CONSOLE --> MADMIN
    CONSOLE --> PKG
    CONSOLE --> OPENAPI
    CONSOLE --> MC_DEP

    MC --> MINIOGO
    MC --> MADMIN
    MC --> PKG
    MC --> COLORJSON
    MC --> FILEPATH
    MC --> SELF
    MC --> PROBE
    MC --> CLI
```

---

## Diagram 4 — Acyclic dependency rule

Demonstrates the one-direction constraint that prevents circular dependencies.

```mermaid
graph TD
    direction TB

    subgraph RULE["Dependency direction rule: always downward, never upward or sideways between T1"]
        T1A["minio/minio"] 
        T1B["minio/mc"]
        T1C["minio/console"]

        T2A["minio/minio-go"]
        T2B["minio/madmin-go"]
        T2C["minio/pkg"]

        T3A["primitives\n(highwayhash, sio, mux, …)"]

        T1A --> T2A
        T1A --> T2B
        T1A --> T2C
        T1B --> T2A
        T1B --> T2B
        T1B --> T2C
        T1C --> T2A
        T1C --> T2B
        T1C --> T2C

        T2A --> T3A
        T2B --> T3A
        T2C --> T3A

        FORBIDDEN1["❌ mc importing minio server\n(would be upward)"]
        FORBIDDEN2["❌ minio-go importing madmin-go\n(would be sideways at T2)"]
        FORBIDDEN3["❌ highwayhash importing minio-go\n(would be upward from T3 to T2)"]
    end
```

---

## Diagram 5 — Current repo lifecycle status

Maps each repo to its current maintenance state.

```mermaid
graph LR
    subgraph MAINTAINED["🟢 Actively maintained (2026)"]
        MINIOGO["minio/minio-go/v7\nApr 2026"]
        MADMIN["minio/madmin-go\nJun 2026"]
        PKG["minio/pkg\nJun 2026"]
        MC["minio/mc\nNov 2025"]
        HWH["minio/highwayhash\nOct 2025"]
        SIO["minio/sio\nNov 2025"]
        ZIP["minio/zipindex\nJan 2026"]
    end

    subgraph FROZEN["🔵 Frozen / stable (public, not archived)"]
        SIMD["minio/simdjson-go\nMar 2023"]
        MUX["minio/mux\nJul 2024"]
        MD5["minio/md5-simd\n~2024"]
        CLI["minio/cli\n~2024"]
        SELF["minio/selfupdate\nlate 2024"]
        DNS["minio/dnscache\n~2023"]
        XXML["minio/xxml\n~2023"]
        CSV["minio/csvparser\n~2021"]
    end

    subgraph GUTTED["🟡 Public but gutted / private activity"]
        CONSOLE["minio/console\nFeb 2025 public\nSep 2025 private"]
    end

    subgraph ARCHIVED["⬛ Archived (read-only)"]
        SERVER["minio/minio\nApr 2026"]
        OPERATOR["minio/operator\n2024"]
        KES["minio/kes\n~2024"]
    end
```
