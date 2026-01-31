# DirIO Configuration Guide

DirIO uses a **dual configuration system** that separates application settings from data directory settings.

## Configuration Architecture

### Application Config (CLI/ENV/YAML)
Controls **how the tool runs** - operational preferences that don't affect data correctness.

**Sources** (priority order):
1. Environment variables (`DIRIO_*`)
2. CLI flags
3. Config file (`~/.dirio/config.yaml`, `/etc/dirio/config.yaml`)
4. Default values

**Settings:**
- `data-dir`: Path to data directory
- `port`: HTTP server port
- `log-level`, `log-format`, `verbosity`, `debug`: Logging preferences
- `mdns-enabled`, `mdns-name`, `mdns-hostname`, `mdns-mode`: Service discovery
- `canonical-domain`: URL generation hint
- `access-key`, `secret-key`: **CLI admin credentials** (temporary/alternative access)

### Data Directory Config (`.dirio/config.json`)
Controls **how data must be handled** - settings that travel with the data and affect correctness.

**Location:** `<data-dir>/.dirio/config.json`

**Settings:**
- `credentials`: **Official admin credentials** for this data directory
- `region`: AWS-style region (e.g., `us-east-1`)
- `compression`: Server-side compression settings
- `wormEnabled`: Write-Once-Read-Many immutability
- `storageClass`: Storage tier configuration

**Created by:**
- MinIO import (automatically from MinIO's `config.json`)
- Migration (from CLI settings on first run)
- Manual creation

## Dual Admin Credentials

DirIO supports **two admin accounts simultaneously**:

### CLI Admin
- Configured via `--access-key` / `--secret-key` flags
- Temporary/alternative admin access
- Good for:
  - Testing
  - Temporary access grants
  - Admin rotation without touching data config

### Data Config Admin
- Stored in `.dirio/config.json`
- Official admin credentials for this data
- Travels with the data directory
- Good for:
  - Production admin account
  - Consistent access across deployments
  - Data-bound authentication

**Both work simultaneously** - no conflicts, no warnings!

## Configuration Precedence

### For Region (and other data-bound settings)
- **Data config takes precedence** if it exists
- CLI flags are **ignored** (with warning logged)
- To update: Edit `.dirio/config.json` manually
  - TODO: `dirio config set region <value>` command (not yet implemented)

### For Credentials
- **Both are valid** - CLI admin AND data admin both work
- No precedence - they coexist peacefully

## Example: Application Config

**File:** `~/.dirio/config.yaml`
```yaml
# How the DirIO tool runs
data_dir: /data
port: 9000

# CLI admin credentials (optional - provides alternative access)
access_key: cli-admin
secret_key: cli-admin-password

# Logging
log_level: info
log_format: text
verbosity: normal

# Service discovery
mdns_enabled: true
mdns_name: dirio-s3

# URL generation
canonical_domain: s3.example.com
```

## Example: Data Config

**File:** `<data-dir>/.dirio/config.json`
```json
{
  "version": "1.0.0",
  "credentials": {
    "accessKey": "production-admin",
    "secretKey": "strong-secret-password"
  },
  "region": "us-east-1",
  "compression": {
    "enabled": false,
    "allowEncryption": false,
    "extensions": [".txt", ".log", ".csv", ".json"],
    "mimeTypes": ["text/*", "application/json"]
  },
  "wormEnabled": false,
  "storageClass": {
    "standard": "",
    "rrs": ""
  }
}
```

## Migration

### New Installation
On first run with an empty data directory:
- Data config is created from CLI flags
- CLI credentials become data config credentials
- Region defaults to `us-east-1`

### Existing Installation (Upgrade)
On first run with existing `.metadata/` but no `.dirio/config.json`:
- Migration automatically creates data config
- CLI credentials are copied to data config
- Log message: "Migrating existing installation"

### MinIO Import
When importing MinIO data:
- Data config is created from MinIO's `config.json`
- Credentials, region, compression, WORM, etc. are preserved
- Saved to `.dirio/config.json` during import

## Environment Variables

All app config settings support environment variables:

```bash
export DIRIO_DATA_DIR=/data
export DIRIO_PORT=9000
export DIRIO_ACCESS_KEY=cli-admin
export DIRIO_SECRET_KEY=cli-password
export DIRIO_LOG_LEVEL=debug
export DIRIO_MDNS_ENABLED=true

dirio serve
```

**Note:** Data config settings (region, compression, etc.) are NOT configurable via environment variables - they must be set in `.dirio/config.json`.

## CLI Flags

```bash
dirio serve \
  --data-dir /data \
  --port 9000 \
  --access-key cli-admin \
  --secret-key cli-password \
  --log-level debug \
  --mdns-enabled
```

**Note:** If data config exists:
- Region and data-bound settings are ignored (logged warning)
- Credentials work alongside data config credentials

## Future: Config Update Commands

**TODO:** Explicit commands to update data config values:
```bash
# Not yet implemented
dirio config set region us-west-2
dirio config set compression.enabled true
dirio config set credentials.accessKey new-admin
```

Currently, edit `.dirio/config.json` manually to update.

## Troubleshooting

### "CLI region flag ignored"
- Data config exists and has a region set
- Your CLI flag is being ignored (this is correct behavior)
- To change: Edit `.dirio/config.json` or wait for `dirio config set` command

### "Two different admin credentials work"
- This is expected! Both CLI and data config admins are valid
- Use whichever is appropriate for your use case

### "Migration didn't create data config"
- Migration only runs if `.metadata/` exists (existing installation)
- New installations create data config on first use
- MinIO imports create data config during import

### "Data config validation failed"
- Check that `.dirio/config.json` has required fields:
  - `version`
  - `credentials.accessKey`
  - `credentials.secretKey`
- See `configs/dataconfig.example.json` for reference
