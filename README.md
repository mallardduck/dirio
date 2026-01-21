# DirIO

**Direct I/O for S3**

DirIO is an S3-compatible object storage server where objects are just files on disk. No chunking, no encoding, no abstraction layers—just directories and files.

Built as a drop-in replacement for MinIO's discontinued single-node filesystem mode.

## Philosophy

- Objects are files. Buckets are directories. That's it.
- No database. Metadata lives in simple JSON files.
- Import your existing MinIO data and keep going.
- One binary. One data directory. Zero ceremony.

## What DirIO Does

- Serves S3 API requests over HTTP
- Stores objects as regular files in `buckets/`
- Maintains metadata in `.metadata/` (JSON files)
- Imports from MinIO's `.minio.sys/` on first boot
- Runs in a container on your NAS

## What DirIO Doesn't Do

- Distributed storage / clustering
- Built-in replication
- Advanced S3 features (yet)
- Replace production-grade S3 implementations

## Quick Start

```bash
# Build
go build -o dirio-server ./cmd/server

# Run
./dirio-server --data-dir /path/to/data --port 9000

# Test
aws --endpoint-url http://localhost:9000 s3 mb s3://test
aws --endpoint-url http://localhost:9000 s3 cp file.txt s3://test/
```

See [QUICKSTART.md](QUICKSTART.md) for detailed setup.

## Status

**Current**: Phase 1 scaffold complete. Core operations implemented but untested.

**Works**: Project structure, storage backend, MinIO import logic  
**TODO**: Testing, auth (Signature V4), advanced features

See [TODO.md](TODO.md) for the full roadmap.

## Architecture

## Directory Layout

```
/data/
├── .metadata/              # DirIO metadata (JSON)
│   ├── users.json         # Credentials
│   ├── policies.json      # Policy definitions  
│   ├── buckets/           # Per-bucket config
│   └── .import-state      # MinIO import tracking
├── .minio.sys/            # MinIO metadata (read-only)
└── buckets/               # Actual objects
    └── mybucket/
        └── path/to/file.jpg
```

Objects in `buckets/` are regular files. Nothing special.

## Supported Operations

**Objects**: GET, PUT, HEAD, DELETE, LIST  
**Buckets**: CREATE, DELETE, HEAD, LIST, GetLocation

That's the MVP. More operations coming in future phases.

## MinIO Migration

## MinIO Migration

Point DirIO at your existing MinIO data directory. It will:

1. Find `.minio.sys/`
2. Import users, bucket policies, and object metadata
3. Write to `.metadata/` (your MinIO files stay untouched)
4. Track import state to detect changes

You can switch back to MinIO anytime. The `buckets/` directory is shared.

## Use Cases

- **Homelab NAS storage**: Host S3 buckets on your NAS without MinIO overhead
- **Static site assets**: Serve website media files via S3 API
- **Backup targets**: Use S3-compatible tools with local filesystem storage
- **Development**: Test S3 integrations locally with real files

## Development

```bash
# Clone and build
git clone https://github.com/yourusername/dirio.git
cd dirio
make build

# Run tests
make test

# Format code  
make fmt
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## Project Structure

- `cmd/server/` - Server binary entry point
- `internal/api/` - S3 API handlers  
- `internal/storage/` - Filesystem operations
- `internal/metadata/` - Metadata + MinIO import
- `internal/auth/` - Authentication (WIP)
- `pkg/s3types/` - S3 types and errors

## License

MIT - See [LICENSE](LICENSE)
