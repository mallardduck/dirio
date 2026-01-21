# Deployment Guide

DirIO is designed to run on a NAS or homelab server. Here's how to deploy it.

## Prerequisites

- Linux host (Ubuntu, Debian, Synology DSM, etc.)
- Docker (optional but recommended)
- Storage directory with adequate space

## Docker Deployment (Recommended)

### Using docker-compose

1. **Create docker-compose.yml**:

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
      - DATA_DIR=/data
      - PORT=9000
      - ACCESS_KEY=minioadmin
      - SECRET_KEY=minioadmin
    restart: unless-stopped
```

2. **Build the image**:

```bash
docker build -t dirio:latest .
```

3. **Start the service**:

```bash
docker-compose up -d
```

### Using Docker CLI

```bash
docker run -d \
  --name dirio \
  -p 9000:9000 \
  -v /path/to/data:/data \
  -e ACCESS_KEY=minioadmin \
  -e SECRET_KEY=minioadmin \
  --restart unless-stopped \
  dirio:latest
```

## Bare Metal Deployment

### Systemd Service

1. **Build the binary**:

```bash
go build -o /usr/local/bin/dirio-server ./cmd/server
```

2. **Create systemd unit** at `/etc/systemd/system/dirio.service`:

```ini
[Unit]
Description=DirIO S3 Server
After=network.target

[Service]
Type=simple
User=dirio
Group=dirio
ExecStart=/usr/local/bin/dirio-server \
  --data-dir /data/dirio \
  --port 9000 \
  --access-key minioadmin \
  --secret-key minioadmin
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

## Synology NAS

### Via Docker (DSM 7.x)

1. Open Container Manager
2. Go to Image → Add → Add from URL
3. Build your image or pull from registry
4. Create container with:
   - Port mapping: 9000 → 9000
   - Volume: `/volume1/Minio/data` → `/data`
   - Environment variables for credentials

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
./dirio-server --data-dir /volume1/Minio/data --port 9000
# Ctrl+A, D to detach
```

## Configuration

### Environment Variables

- `DATA_DIR`: Path to data directory (default: `/data`)
- `PORT`: HTTP port (default: `9000`)
- `ACCESS_KEY`: Root access key (default: `minioadmin`)
- `SECRET_KEY`: Root secret key (default: `minioadmin`)

### Command Line Flags

```bash
dirio-server \
  --data-dir /path/to/data \
  --port 9000 \
  --access-key your-key \
  --secret-key your-secret
```

Flags override environment variables.

## Reverse Proxy (Optional)

### Nginx

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

### Caddy

```
s3.example.com {
    reverse_proxy localhost:9000
}
```

## Monitoring

### Health Check

```bash
curl http://localhost:9000/
# Should return XML with bucket list
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

## Backup

DirIO data is just files. Back up the data directory:

```bash
# Rsync to backup location
rsync -av /data/dirio/ /backup/dirio/

# Or use your NAS's built-in backup tools
```

**Important**: Back up both `buckets/` and `.metadata/` directories.

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
dirio-server --data-dir /volume1/Minio/data
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

# Try manual import (future feature)
dirio-server --import-minio --data-dir /data
```

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

## Security

### Change Default Credentials

```bash
# Set via environment
export ACCESS_KEY="your-strong-key"
export SECRET_KEY="your-strong-secret"

# Or via command line
dirio-server --access-key your-key --secret-key your-secret
```

### Restrict Network Access

```bash
# Firewall rules (iptables)
iptables -A INPUT -p tcp --dport 9000 -s 192.168.1.0/24 -j ACCEPT
iptables -A INPUT -p tcp --dport 9000 -j DROP

# Or use Docker network isolation
```

### Use HTTPS

Always run behind a reverse proxy with TLS for production use.

## Common Patterns

### Internal Homelab Use

```yaml
# docker-compose.yml
services:
  dirio:
    image: dirio:latest
    network_mode: host  # Easy access from all homelab services
    volumes:
      - /mnt/storage/s3:/data
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
