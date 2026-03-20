# DirIO - Filesystem-first S3

**Objects on the outside. Directories on the inside.
A filesystem-native S3-compatible object store.**

<p align="center">
  <img src="./assets/dirio_logo.svg" alt="DirIO Logo" width="200">
</p>

DirIO is an S3-compatible object storage server where objects are just files on disk. No chunking, no encoding, no abstraction layers—just directories and files.

Built as a drop-in replacement for MinIO's discontinued single-node filesystem mode.

## Philosophy

- Objects are files. Buckets are directories. That's it.
- No database. Metadata lives in simple JSON files.
- One binary. One data directory. Zero ceremony.
- Import your existing MinIO data and keep going.

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

## Use Cases

- **Homelab NAS storage**: Host S3 buckets on your NAS without MinIO overhead
- **Static site assets**: Serve website media files via S3 API
- **Backup targets**: Use S3-compatible tools with local filesystem storage
- **Development**: Test S3 integrations locally with real files

## Supported Operations

| Category | Operations |
| -------- | ---------- |
| Objects  | GET, PUT, HEAD, DELETE, LIST, COPY, Multipart Upload |
| Buckets  | CREATE, DELETE, HEAD, LIST, GetLocation, Policy (GET/PUT/DELETE) |
| Metadata | Custom metadata, Object tagging |
| Advanced | Presigned URLs, Range requests |

## IAM & Authorization

DirIO supports S3 bucket policies and user/group management via the MinIO Admin API:

- ✅ S3 bucket policies (AWS CLI, boto3, MinIO `mc`)
- ✅ User & policy management via `mc admin`
- ❌ AWS IAM API (`aws iam`) — not supported
- ❌ Terraform AWS provider — requires AWS IAM API

See [docs/IAM-ARCHITECTURE.md](docs/design/IAM-ARCHITECTURE.md) for details.

## MinIO Migration

Point DirIO at your existing MinIO data directory. It will:

1. Find `.minio.sys/`
2. Import users, bucket policies, and object metadata
3. Write to `.metadata/` (your MinIO files stay untouched)
4. Track import state to detect changes

You can switch back to MinIO anytime. The `buckets/` directory is shared.

## Contributing & Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution guidelines and [DEVELOPMENT.md](docs/DEVELOPMENT.md) for project structure, build setup, and architecture notes.

## License

MIT - See [LICENSE](LICENSE)
