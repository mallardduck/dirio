# Quick Start Guide

Get DirIO running in 5 minutes.

## Extract and Build

```bash
tar -xzf dirio.tar.gz
cd dirio
go mod tidy
go build -o dirio-server ./cmd/server
```

## Run

```bash
./dirio-server --data-dir ./data --port 9000
```

DirIO is now running on http://localhost:9000

## Test with AWS CLI

```bash
# Configure (one time)
aws configure set aws_access_key_id minioadmin
aws configure set aws_secret_access_key minioadmin

# Create a bucket
aws --endpoint-url http://localhost:9000 s3 mb s3://test

# Upload a file
echo "Hello DirIO" > test.txt
aws --endpoint-url http://localhost:9000 s3 cp test.txt s3://test/

# List objects
aws --endpoint-url http://localhost:9000 s3 ls s3://test/

# Download
aws --endpoint-url http://localhost:9000 s3 cp s3://test/test.txt downloaded.txt

# Verify
cat downloaded.txt  # Should print "Hello DirIO"
```

Your file is now at: `data/buckets/test/test.txt`

Check it: `cat data/buckets/test/test.txt`

## Run with Docker

```bash
docker build -t dirio:latest .
docker run -d -p 9000:9000 -v $(pwd)/data:/data dirio:latest
```

Or use docker-compose:

```bash
docker-compose up -d
```

## Migrate from MinIO

1. **Stop MinIO** (important!)

```bash
docker stop minio
```

2. **Point DirIO at MinIO data**

```bash
./dirio-server --data-dir /path/to/minio/data --port 9000
```

3. **Check logs for import**

```
Detected MinIO data. Starting import...
Imported user: rancher
Imported bucket: mybucket
MinIO import completed successfully
```

4. **Verify**

```bash
aws --endpoint-url http://localhost:9000 s3 ls
```

Your buckets and objects are now available via DirIO.

## What's Created

When DirIO starts, it creates:

```
data/
├── .metadata/              # DirIO metadata (JSON)
│   ├── users.json
│   ├── buckets/
│   └── .import-state
├── .minio.sys/            # MinIO metadata (if migrating)
└── buckets/               # Your objects
    └── test/
        └── test.txt       # Regular file!
```

## Next Steps

- See [DEPLOYMENT.md](docs/DEPLOYMENT.md) for production setup
- See [FAQ.md](docs/FAQ.md) for common questions  
- See [TODO.md](TODO.md) for current status
- See [DESIGN.md](docs/DESIGN.md) for architecture

## Troubleshooting

**Port in use:**
```bash
lsof -i :9000  # Find what's using the port
```

**Permission errors:**
```bash
chmod -R 755 data/
```

**Import failed:**
- Ensure `.minio.sys/` exists in data directory
- Check logs: `docker logs dirio`
- Verify MinIO version is `RELEASE.2022-10-24T18-35-07Z` or earlier

**Can't connect:**
- Check firewall: `sudo ufw allow 9000`
- Verify DirIO is running: `curl http://localhost:9000/`

## Help

- Read [FAQ.md](docs/FAQ.md)
- Check [TODO.md](TODO.md) for known issues
- Open an issue on GitHub

