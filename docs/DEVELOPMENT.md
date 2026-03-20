# DirIO — Development Notes

## Project Status

**Current phase: 4.5 complete.** Phases 1–4.5 are done. Next up: Phase 5 (observability & health) and Phase 8 (extended console UI).

See [TODO.md](../TODO.md) for the full roadmap and [docs/CHANGELOG.md](docs/CHANGELOG.md) for completed work.

## Tooling

This project uses [go-task](https://taskfile.dev) for build automation.

```bash
# Install go-task (once)
go install github.com/go-task/task/v3/cmd/task@latest

# Build
task build

# Run tests
task test

# Format code
task fmt

# See all available tasks
task --list
```

## Running Locally

```bash
./dirio-server --data-dir /path/to/data --port 9000
```

See [docs/configuration.md](dev/configuration.md) for all CLI flags, env vars, and config file options.

## Project Structure

```
cmd/server/         — Server binary entry point
console/            — Web admin console (templ + HTMX + Tailwind)
consoleapi/         — Console API interface (seam between console and server)
internal/
  auth/             — AWS Signature V4 authentication
  config/           — App configuration (Cobra/Viper)
  console/          — Console adapter (wires interface to service layer)
  dataconfig/       — Data directory config (.dirio/config.json)
  http/
    api/iam/        — MinIO Admin API handlers
    api/s3/         — S3 API handlers
    auth/           — Auth middleware (multi-user credential lookup)
    server/         — Router and server setup
  mdns/             — mDNS service discovery
  metadata/         — Metadata manager + MinIO import
  path/             — go-billy filesystem abstraction
  policy/           — Policy evaluation engine
  storage/          — Filesystem operations
pkg/
  iam/              — IAM types (users, policies, service accounts, groups)
  s3types/          — S3 types and errors
```

## Data Directory Layout

```
/data/
├── .dirio/
│   ├── config.json            # Instance config (region, credentials, compression)
│   ├── metadata/              # BoltDB metadata store
│   │   ├── users/             # User records (keyed by UUID)
│   │   ├── policies/          # IAM policy records
│   │   ├── buckets/           # Per-bucket metadata
│   │   ├── objects/           # Per-object metadata
│   │   ├── groups/            # Group records
│   │   ├── service-accounts/  # Service account records
│   │   └── .import-state      # MinIO import phase tracking
│   └── keyring/               # Encryption keys
├── .minio.sys/                # MinIO metadata (read-only, import source)
└── <bucket-name>/             # Object files (regular filesystem files)
    └── path/to/object
```

Objects are plain files — no special encoding. The `.dirio/` directory is the only DirIO-managed state.

## Architecture Notes

- **Single binary, single data directory.** No external databases.
- **Filesystem-first.** Objects are real files; metadata is JSON in `.dirio/`.
- **Dual-port optional.** S3 on `:9000`, admin+console on `:9001`. Single-port is the default.
- **Build tag `noconsole`** strips the web console entirely: `go build -tags noconsole`.
- **`consoleapi/`** is the only coupling point between the console and the rest of the server — the console package is designed to be extractable.

See [docs/IAM-ARCHITECTURE.md](design/IAM-ARCHITECTURE.md) and [docs/CONSOLE-ARCHITECTURE.md](design/CONSOLE-ARCHITECTURE.md) for deeper dives.

## Testing

```bash
# Unit + integration tests
task test

# Run against a live server with AWS CLI
aws --endpoint-url http://localhost:9000 s3 ls

# Client compatibility tests (boto3, mc, AWS CLI)
# See tests/clients/README.md
```

Fixed bugs are tracked in [bugs/fixed/](bugs/fixed/).