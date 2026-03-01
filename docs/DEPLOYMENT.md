# Deployment Guide

DirIO is designed to run on a NAS or homelab server. Here's how to deploy it.

## Prerequisites

- Linux host (Ubuntu, Debian, Synology DSM, etc.)
- Docker (optional but recommended)
- Storage directory with adequate space

## Deployment Modes

DirIO supports two port topologies. Both are fully supported; choose based on your needs.

### Single-Port Mode (default)

The S3 data plane, MinIO-compatible admin API, and web console all share one port (`:9000`). DirIO-specific routes are served under the `/.dirio/` path prefix on the same port.

**Best for:** development, embedded deployments, or simple homelab setups where firewall rules aren't a concern.

**Trade-off:** when using path-style bucket routing, `/.dirio` is reserved and cannot be used as a bucket name.

```
:9000  →  S3 API (path-style: /{bucket}/{key})
           MinIO admin API (/minio/admin/v3/...)
           Web console (/dirio/ui/)
           Health/metrics (/.dirio/health, /.dirio/metrics)
```

### Dual-Port Mode (recommended for production)

The S3 data plane runs on a dedicated port (`:9000`), while the admin API and web console run on a separate control-plane port (`:9010`). Each listener has its own router with no path-prefix logic.

**Best for:** production deployments, Synology NAS, or any setup where you want clean separation between data and control traffic.

**Benefits:**
- Firewall rules become trivial: block `:9010` from external access while keeping `:9000` open for S3 clients
- The S3 port has no reserved paths — no bucket name conflicts in path-style routing
- Clean DNS split: point `s3.myserver.local → :9000` and `admin.myserver.local → :9010`

```
:9000  →  S3 API only (path-style: /{bucket}/{key})
:9010  →  MinIO admin API (/minio/admin/v3/...)
           Web console (/dirio/ui/)
           Health/metrics (/.dirio/health, /.dirio/metrics)
```

Enable dual-port mode with:

```bash
dirio serve --console-dedicated-port --console-port 9010
```

Or via environment:

```bash
DIRIO_CONSOLE_DEDICATED_PORT=true DIRIO_CONSOLE_PORT=9010 dirio serve
```

---

## Docker Deployment (Recommended)

### Single-Port Mode (docker-compose)

```yaml
version: '3.8'

services:
  dirio:
    image: dirio:latest
    container_name: dirio
    ports:
      - "9000:9000"
    volumes:
      - /volume1/Minio/data:/data  # Point to your data directory
    environment:
      - DIRIO_DATA_DIR=/data
      - DIRIO_PORT=9000
      - DIRIO_ACCESS_KEY=your-access-key
      - DIRIO_SECRET_KEY=your-secret-key
    restart: unless-stopped
```

### Dual-Port Mode (docker-compose, recommended for production)

```yaml
version: '3.8'

services:
  dirio:
    image: dirio:latest
    container_name: dirio
    ports:
      - "9000:9000"   # S3 data plane — expose to S3 clients
      - "9010:9010"   # Admin/console control plane — restrict as needed
    volumes:
      - /volume1/Minio/data:/data
    environment:
      - DIRIO_DATA_DIR=/data
      - DIRIO_PORT=9000
      - DIRIO_ACCESS_KEY=your-access-key
      - DIRIO_SECRET_KEY=your-secret-key
      - DIRIO_CONSOLE_DEDICATED_PORT=true
      - DIRIO_CONSOLE_PORT=9010
    restart: unless-stopped
```

### Build the image

```bash
docker build -t dirio:latest .
```

### Start the service

```bash
docker-compose up -d
```

### Using Docker CLI

**Single-port:**

```bash
docker run -d \
  --name dirio \
  -p 9000:9000 \
  -v /path/to/data:/data \
  -e DIRIO_ACCESS_KEY=your-access-key \
  -e DIRIO_SECRET_KEY=your-secret-key \
  --restart unless-stopped \
  dirio:latest
```

**Dual-port:**

```bash
docker run -d \
  --name dirio \
  -p 9000:9000 \
  -p 9010:9010 \
  -v /path/to/data:/data \
  -e DIRIO_ACCESS_KEY=your-access-key \
  -e DIRIO_SECRET_KEY=your-secret-key \
  -e DIRIO_CONSOLE_DEDICATED_PORT=true \
  -e DIRIO_CONSOLE_PORT=9010 \
  --restart unless-stopped \
  dirio:latest
```

---

## Bare Metal Deployment

### Systemd Service

1. **Build the binary**:

```bash
go build -o /usr/local/bin/dirio-server ./cmd/server
```

2. **Create systemd unit** at `/etc/systemd/system/dirio.service`:

**Single-port:**

```ini
[Unit]
Description=DirIO S3 Server
After=network.target

[Service]
Type=simple
User=dirio
Group=dirio
ExecStart=/usr/local/bin/dirio-server serve \
  --data-dir /data/dirio \
  --port 9000 \
  --access-key your-access-key \
  --secret-key your-secret-key
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

**Dual-port** (add `--console-dedicated-port`):

```ini
[Unit]
Description=DirIO S3 Server
After=network.target

[Service]
Type=simple
User=dirio
Group=dirio
ExecStart=/usr/local/bin/dirio-server serve \
  --data-dir /data/dirio \
  --port 9000 \
  --access-key your-access-key \
  --secret-key your-secret-key \
  --console-dedicated-port \
  --console-port 9010
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

3. **Create user and directory**:

```bash
useradd -r -s /bin/false dirio
mkdir -p /data/dirio
chown -R dirio:dirio /data/dirio
```

4. **Enable and start**:

```bash
systemctl daemon-reload
systemctl enable dirio
systemctl start dirio
```

---

## Synology NAS

### Via Docker (DSM 7.x)

1. Open Container Manager
2. Go to Image → Add → Add from URL
3. Build your image or pull from registry
4. Create container with:
   - Port mapping: 9000 → 9000 (S3), 9010 → 9010 (admin, dual-port mode)
   - Volume: `/volume1/Minio/data` → `/data`
   - Environment variables for credentials (see Configuration below)

### Via SSH (Advanced)

```bash
# SSH into your NAS
ssh admin@nas.local

# Build DirIO
cd /volume1/docker/dirio
git clone <repo>
cd dirio
go build -o dirio-server ./cmd/server

# Run in screen or tmux
screen -S dirio
./dirio-server serve --data-dir /volume1/Minio/data --port 9000
# Ctrl+A, D to detach
```

---

## Configuration

### Environment Variables

All environment variables use the `DIRIO_` prefix.

| Variable | Default | Description |
|---|---|---|
| `DIRIO_DATA_DIR` | `/data` | Path to data directory |
| `DIRIO_PORT` | `9000` | S3 HTTP port |
| `DIRIO_ACCESS_KEY` | `dirio-admin` | Root access key |
| `DIRIO_SECRET_KEY` | `dirio-admin-secret` | Root secret key |
| `DIRIO_CONSOLE` | `true` | Enable the web admin console |
| `DIRIO_CONSOLE_DEDICATED_PORT` | `false` | Enable dual-port mode |
| `DIRIO_CONSOLE_PORT` | `9010` | Admin/console port (dual-port mode only) |
| `DIRIO_LOG_LEVEL` | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `DIRIO_LOG_FORMAT` | `text` | Log format (`text`, `json`) |
| `DIRIO_MDNS_ENABLED` | `false` | Enable mDNS service discovery |
| `DIRIO_CANONICAL_DOMAIN` | `` | Canonical domain for URL generation |

### Command Line Flags

```bash
# Single-port mode
dirio serve \
  --data-dir /path/to/data \
  --port 9000 \
  --access-key your-key \
  --secret-key your-secret

# Dual-port mode
dirio serve \
  --data-dir /path/to/data \
  --port 9000 \
  --access-key your-key \
  --secret-key your-secret \
  --console-dedicated-port \
  --console-port 9010
```

Flags override environment variables, which override config file values.

---

## Reverse Proxy (Optional)

### Nginx — Single-Port Mode

```nginx
server {
    listen 80;
    server_name s3.example.com;

    location / {
        proxy_pass http://localhost:9000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Disable buffering for large uploads
        proxy_request_buffering off;
        proxy_buffering off;

        # Increase timeouts for large files
        proxy_connect_timeout 300;
        proxy_send_timeout 300;
        proxy_read_timeout 300;
        send_timeout 300;
    }
}
```

### Nginx — Dual-Port Mode

```nginx
# S3 data plane — exposed to S3 clients
server {
    listen 80;
    server_name s3.example.com;

    location / {
        proxy_pass http://localhost:9000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        proxy_request_buffering off;
        proxy_buffering off;

        proxy_connect_timeout 300;
        proxy_send_timeout 300;
        proxy_read_timeout 300;
        send_timeout 300;
    }
}

# Admin/console control plane — restrict to internal network
server {
    listen 80;
    server_name admin.example.com;

    # Restrict to internal network only
    allow 192.168.1.0/24;
    deny all;

    location / {
        proxy_pass http://localhost:9010;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### Caddy — Single-Port

```
s3.example.com {
    reverse_proxy localhost:9000
}
```

### Caddy — Dual-Port

```
s3.example.com {
    reverse_proxy localhost:9000
}

admin.example.com {
    reverse_proxy localhost:9010
}
```

---

## Monitoring

### Health Check

```bash
# Single-port mode
curl http://localhost:9000/.dirio/health
# {"status":"ok","storage":"ok","metadata":"ok"}

# Dual-port mode (health on control plane port)
curl http://localhost:9010/.dirio/health
```

### Metrics

```bash
# Single-port mode
curl http://localhost:9000/.dirio/metrics

# Dual-port mode
curl http://localhost:9010/.dirio/metrics
```

### Logs

**Docker**:
```bash
docker logs -f dirio
```

**Systemd**:
```bash
journalctl -u dirio -f
```

### Disk Usage

```bash
# Check data directory size
du -sh /data/dirio/buckets
```

---

## Backup

DirIO data is just files. Back up the data directory:

```bash
# Rsync to backup location
rsync -av /data/dirio/ /backup/dirio/
```

**Important**: Back up both `buckets/` and `.metadata/` directories.

---

## Upgrading

### Docker

```bash
# Pull new image
docker pull dirio:latest

# Recreate container
docker-compose down
docker-compose up -d
```

### Bare Metal

```bash
# Stop service
systemctl stop dirio

# Replace binary
go build -o /usr/local/bin/dirio-server ./cmd/server

# Start service
systemctl start dirio
```

**Note**: Always backup data before upgrading.

---

## Migration from MinIO

1. **Stop MinIO**:
```bash
docker stop minio
# or
systemctl stop minio
```

2. **Point DirIO at MinIO data**:
```bash
# Docker
docker run -d \
  -p 9000:9000 \
  -v /volume1/Minio/data:/data \
  dirio:latest

# Bare metal
dirio serve --data-dir /volume1/Minio/data
```

3. **Verify import**:
```bash
# Check logs for import messages
docker logs dirio | grep -i import

# Test bucket access
aws --endpoint-url http://localhost:9000 s3 ls
```

4. **Keep MinIO container** for rollback:
```bash
# Don't delete the MinIO image yet
# Just keep it stopped for a week
```

---

## Troubleshooting

### Port Already in Use

```bash
# Check what's using port 9000
sudo lsof -i :9000

# Stop conflicting service
docker stop minio  # or other service
```

### Permission Denied

```bash
# Fix data directory permissions
chown -R dirio:dirio /data/dirio

# Or for Docker
chown -R 1000:1000 /data/dirio
```

### Import Failed

```bash
# Check if .minio.sys exists
ls -la /data/.minio.sys

# Check logs for specific error
docker logs dirio | grep -i error
```

---

## Performance Tuning

### Filesystem

- Use XFS or ext4 for large file performance
- Enable noatime mount option to reduce disk writes
- Consider SSD for metadata if you have many small files

### Network

- Use 10GbE if transferring large files
- Disable TCP offloading if you see corruption: `ethtool -K eth0 tx off rx off`

### Limits

- Increase ulimit if handling many concurrent connections
- Adjust Docker memory limits if needed

---

## Security

### Change Default Credentials

```bash
# Set via environment
export DIRIO_ACCESS_KEY="your-strong-key"
export DIRIO_SECRET_KEY="your-strong-secret"

# Or via command line
dirio serve --access-key your-key --secret-key your-secret
```

### Restrict Network Access

In dual-port mode, firewall only the admin port — the S3 port can remain open:

```bash
# Allow S3 from anywhere, admin only from LAN
iptables -A INPUT -p tcp --dport 9000 -j ACCEPT
iptables -A INPUT -p tcp --dport 9010 -s 192.168.1.0/24 -j ACCEPT
iptables -A INPUT -p tcp --dport 9010 -j DROP

# Or use Docker network isolation
```

### Use HTTPS

Always run behind a reverse proxy with TLS for production use.

---

## Common Patterns

### Internal Homelab — Single-Port

```yaml
# docker-compose.yml
services:
  dirio:
    image: dirio:latest
    network_mode: host  # Easy access from all homelab services
    volumes:
      - /mnt/storage/s3:/data
    environment:
      - DIRIO_PORT=9000
```

### Internal Homelab — Dual-Port (Recommended)

```yaml
# docker-compose.yml
services:
  dirio:
    image: dirio:latest
    ports:
      - "9000:9000"
      - "9010:9010"
    volumes:
      - /mnt/storage/s3:/data
    environment:
      - DIRIO_PORT=9000
      - DIRIO_CONSOLE_DEDICATED_PORT=true
      - DIRIO_CONSOLE_PORT=9010
```

### Public Read Buckets

```bash
# Set bucket policy to public-read (future feature)
# For now, use reverse proxy with authentication
```

### Multiple Data Directories

Not supported. Use symlinks if needed:

```bash
ln -s /mnt/disk2/more-buckets /data/dirio/buckets/extra
```