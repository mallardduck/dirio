# DirIO V2: Hybrid Storage & Erasure Coding

> **Status:** Planning / Future Work
> V1 implements the Standard driver only. The StorageCoordinator abstraction and Sidecar EC driver are future phases. **For most users today, the recommendation is to use ZFS or BTRFS and let the filesystem handle durability.**

---

## 1. Core Philosophy

- **Transparency:** Buckets are regular directories. Objects are regular files. `ls`, `cp`, and `cat` always work on your data.
- **Hybrid Durability:** Native filesystem protection (ZFS/BTRFS) is the preferred path. Application-managed Sidecar EC is available for environments where that isn't an option.
- **Zero Lock-in:** If DirIO is removed, data remains intact and organised on the primary disk.
- **Pure Go:** No Cgo dependencies. Reed-Solomon math via `klauspost/reedsolomon`.

---

## 2. Storage Architecture

A `StorageCoordinator` abstracts the physical location of data and parity, presenting a unified interface to the S3 API layer.

### A. Data Tier (Primary Disk)

The primary mount point. Files are stored 1:1 with the S3 object path. This disk is always human-readable.

```
/mnt/disk_main/
└── movies/
    └── gopher.mp4        ← the actual file, accessible directly
└── .metadata/
    └── movies/
        └── gopher.mp4.json   ← ETag, content-type, checksum, EC manifest ref
```

Object metadata (content-type, ETag, custom headers) is stored in DirIO's existing `.metadata/` layer, consistent with V1 behaviour. Extended attributes (xattrs) are used where available as a secondary integrity hint but are never the sole source of truth, as xattr support varies across filesystems (notably absent or limited on NTFS, some NFS mounts, and network-attached storage).

### B. Parity Tier (Secondary Disks)

When Sidecar EC is enabled, parity shards are calculated and distributed across one or more secondary physical disks. These disks are internal to DirIO and not intended for direct user access.

```
/mnt/disk_parity_1/
└── .dirio/
    └── movies/
        └── gopher.mp4.p1   ← 1st parity shard

/mnt/disk_parity_2/
└── .dirio/
    └── movies/
        └── gopher.mp4.p2   ← 2nd parity shard
```

**Naming convention:** `1+N` — one human-readable data file plus N parity shards. The data file on disk is the canonical data source; it is not split or encoded. Parity shards exist solely to reconstruct it if it becomes corrupted or lost.

---

## 3. Driver Model

The `StorageCoordinator` selects a driver based on configuration and environment detection.

| Driver | Use Case | Mechanism |
|---|---|---|
| **Standard** | Single disk / development | No redundancy. Simple file storage. Current V1 behaviour. |
| **Native Pass-through** | ZFS, BTRFS, Hardware RAID | Standard `os` calls. Filesystem handles bit-rot repair and redundancy. No parity overhead. |
| **Sidecar EC** | Ext4, XFS, NTFS, APFS, any JBOD | DirIO generates N parity shards on separate disks. Manages reconstruction in-app. |

The Native Pass-through driver is the **recommended production path** for V2. Sidecar EC is intended for environments where a capable filesystem is not available or practical.

---

## 4. IO Flow

### Write Path (PUT)

To maintain performance, DirIO uses a parallelised streaming approach rather than writing data and then computing parity sequentially.

1. **Incoming stream** — S3 API receives the object body.
2. **TeeReader split** — the stream is forked. One branch writes to the Data Tier. The other feeds the RS encoder.
3. **Parallel flush** — parity shards are written to each parity disk simultaneously via `errgroup`. The PUT returns only after both the data write and all parity writes confirm success.

### Read Path (GET)

- **Healthy read:** Data file is present and checksum matches. Served directly via `open()` — no reconstruction math involved. This is the common case and has no overhead vs. V1.
- **Degraded read:** Data file is missing or checksum fails. Parity shards are used to reconstruct the object in-memory. The reconstructed data is served to the client and optionally healed back to disk (see Scrubber below).

---

## 5. Integrity & Metadata

- **Checksums:** SHA-256 or HighwayHash digests are stored in DirIO's `.metadata/` layer alongside existing object metadata. xattrs may be written as a supplementary hint on filesystems that support them, but `.metadata/` is authoritative.
- **Parity manifest:** A small sidecar JSON file accompanies each parity set, recording the EC configuration (k+m values, shard count, algorithm version) used when the object was written. This allows safe reconstruction even if global config changes later.

---

## 6. Background Scrubber

A background goroutine crawls the Data Tier on a configurable schedule:

1. Reads each object and verifies its checksum against `.metadata/`.
2. On mismatch, attempts reconstruction from parity shards.
3. If reconstruction succeeds, heals the data file back to disk and logs the event.
4. If reconstruction fails (insufficient shards), marks the object as degraded and raises an alert.

The scrubber runs at low priority and is IO-throttled to avoid impacting foreground request performance.

---

## 7. Full Layout Example (`1+2` setup)

```
/mnt/disk_main/                     ← User-facing
└── movies/
    └── gopher.mp4                  ← The actual video file
└── .metadata/
    └── movies/
        └── gopher.mp4.json         ← ETag, content-type, checksum, EC manifest ref

/mnt/disk_parity_1/                 ← Internal (DirIO-managed)
└── .dirio/
    └── movies/
        └── gopher.mp4.p1

/mnt/disk_parity_2/                 ← Internal (DirIO-managed)
└── .dirio/
    └── movies/
        └── gopher.mp4.p2
```

---

## 8. Configuration

EC is opt-in and configured per-deployment. A minimal example:

```yaml
storage:
  driver: sidecar_ec       # standard | native_passthrough | sidecar_ec
  data_dir: /mnt/disk_main
  parity_dirs:
    - /mnt/disk_parity_1
    - /mnt/disk_parity_2
  ec:
    parity_shards: 2        # N in 1+N
    checksum: highwayhash   # sha256 | highwayhash
  scrubber:
    enabled: true
    interval: 24h
    io_limit_mbps: 50
```

---

## 9. Key Constraints & Tradeoffs

- **Write amplification:** A `1+2` setup writes ~3× the data of a standard write. On spinning disks with slow parity drives this will be visible. Benchmark before deploying on constrained hardware.
- **No stripe splitting:** The data file is always a complete, intact copy of the object. There is no performance gain from distributing reads across shards (unlike striped RAID). The tradeoff is explicit: readability and zero-lock-in over read throughput.
- **Single-file objects only:** Multipart uploads are reassembled before parity is calculated. Parity is computed on the complete object, not individual parts.
- **Sidecar EC does not replace a backup.** It protects against bit-rot and single-disk failure. It does not protect against accidental deletion, ransomware, or whole-node failure.

---

## 10. V2 Roadmap Position

| Phase | Item |
|---|---|
| V1 (current) | Standard driver, `.metadata/` layer, mDNS, S3 core API, IAM, console |
| V2 milestone 1 | `StorageCoordinator` interface + driver abstraction layer |
| V2 milestone 2 | Sidecar EC write path (TeeReader + parallel flush) |
| V2 milestone 3 | Degraded read + in-memory reconstruction |
| V2 milestone 4 | Scrubber + heal-to-disk |
| V2 milestone 5 | Parity manifest + config versioning |

See [TODO.md](../../TODO.md) for the full task breakdown.
