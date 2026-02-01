# DirIO S3 Test Setup Scripts

This directory contains scripts to set up test data on S3-compatible storage systems for debugging and testing.

## Scripts Overview

### Original Docker-based Scripts

- **`minio-standalone-setup.sh`** - Sets up a MinIO 2022 instance in Docker with legacy FS mode
- **`minio-2019-setup.sh`** - Sets up a MinIO 2019 instance in Docker (FS mode "golden era")
- **`minio-import-2019-to-2022.sh`** - Imports 2019 data into a 2022 MinIO instance for migration testing

### Generic S3 Setup Scripts (New!)

- **`s3-generic-setup.py`** - Python script using boto3 (recommended, truly generic)
- **`s3-generic-setup.sh`** - Bash script for Linux/macOS/WSL (uses MinIO client)
- **`s3-generic-setup.ps1`** - PowerShell script for Windows (uses MinIO client)

These generic scripts can point at **any** S3-compatible API endpoint and create test data. This is useful for:

- Setting up known test state on your DirIO server
- Testing client-specific bugs by ensuring consistent server state
- Debugging without being tied to a specific MinIO Docker setup
- Testing against AWS S3, MinIO, DirIO, or any other S3-compatible service

## Generic Script Usage

### Prerequisites

**Option 1: Python with boto3 (Recommended)**

This is the most generic option and works across all platforms:

```bash
# Install boto3
pip install boto3
```

**Option 2: MinIO client (`mc`)**

If you prefer bash/PowerShell scripts:

**Linux/macOS:**
```bash
# macOS
brew install minio/stable/mc

# Linux
wget https://dl.min.io/client/mc/release/linux-amd64/mc
chmod +x mc
sudo mv mc /usr/local/bin/
```

**Windows:**
Download from https://dl.min.io/client/mc/release/windows-amd64/mc.exe and add to PATH.

### Basic Usage

**Python (All platforms - Recommended):**
```bash
cd scripts

# Set environment variables
export S3_ENDPOINT="http://localhost:9000"
export S3_ACCESS_KEY="dirio-admin"
export S3_SECRET_KEY="dirio-admin-secret"

# Run the script
python3 s3-generic-setup.py
```

**Linux/macOS/WSL (Bash):**
```bash
cd scripts

# Set environment variables
export S3_ENDPOINT="http://localhost:8080"
export S3_ACCESS_KEY="admin"
export S3_SECRET_KEY="password123"

# Run the script
./s3-generic-setup.sh
```

**Windows (PowerShell):**
```powershell
cd scripts

# Set environment variables
$env:S3_ENDPOINT = "http://localhost:8080"
$env:S3_ACCESS_KEY = "admin"
$env:S3_SECRET_KEY = "password123"

# Run the script
.\s3-generic-setup.ps1
```

### Which Script Should I Use?

**Use the Python script (`s3-generic-setup.py`)** if:
- You want the most generic, S3-standard implementation
- You're working across multiple platforms
- You need SSL verification control
- You have Python 3 and can install boto3

**Use the Bash script (`s3-generic-setup.sh`)** if:
- You're on Linux/macOS/WSL
- You already have the MinIO client installed
- You prefer shell scripting

**Use the PowerShell script (`s3-generic-setup.ps1`)** if:
- You're on Windows without WSL
- You already have the MinIO client installed
- You prefer PowerShell

All three scripts create the same test data and have the same functionality.

### Configuration Options

All scripts support these environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `S3_ENDPOINT` | *(required)* | S3 API endpoint URL (e.g., `http://localhost:8080`) |
| `S3_ACCESS_KEY` | *(required)* | Access key / username |
| `S3_SECRET_KEY` | *(required)* | Secret key / password |
| `S3_ALIAS` | `target` | mc alias name for this endpoint (bash/PS only) |
| `S3_REGION` | `us-east-1` | AWS region (if needed) |
| `OBJECT_SIZE` | `65536` | Size of test objects in bytes |
| `SKIP_USERS` | `false` | Set to `true` to skip IAM user creation |
| `SKIP_POLICIES` | `false` | Set to `true` to skip bucket policy creation |
| `VERIFY_SSL` | `true` | Set to `false` to disable SSL verification (Python only) |

### Example: Testing Against DirIO

```bash
# Start your DirIO server on port 8080
cd dirio
go run cmd/dirio/main.go

# In another terminal, run the setup script
cd scripts
export S3_ENDPOINT="http://localhost:8080"
export S3_ACCESS_KEY="your-access-key"
export S3_SECRET_KEY="your-secret-key"
python3 s3-generic-setup.py
```

### Example: Skip IAM (for servers without IAM support)

```bash
export S3_ENDPOINT="http://localhost:8080"
export S3_ACCESS_KEY="admin"
export S3_SECRET_KEY="password123"
export SKIP_USERS="true"  # Don't try to create IAM users
python3 s3-generic-setup.py
```

### Example: Self-Signed SSL Certificate

If your S3 endpoint uses a self-signed certificate:

```bash
export S3_ENDPOINT="https://localhost:8443"
export S3_ACCESS_KEY="admin"
export S3_SECRET_KEY="password123"
export VERIFY_SSL="false"  # Disable SSL verification (Python script only)
python3 s3-generic-setup.py
```

## What Gets Created

The generic setup scripts create:

### Buckets
- **`alpha`** - Private bucket (alice user if IAM supported)
- **`beta`** - Public-read bucket (if bucket policies supported)
- **`gamma`** - Public-read bucket (if bucket policies supported)

### IAM Users (if not skipped)
- **`alice`** / `alicepass1234` - Read/write access to `alpha` bucket
- **`bob`** / `bobpass1234` - Read/write access to `beta` bucket

### Objects

**Basic objects:**
- `alpha/alice-object.bin` (64KB random data)
- `beta/bob-object.bin` (64KB random data)
- `gamma/public-object.bin` (64KB random data)

**Folder structure (for ListObjects delimiter testing):**
- `alpha/folder1/file1.txt`
- `alpha/folder1/file2.txt`
- `alpha/folder1/subfolder/deep.txt`
- `alpha/folder2/file1.txt`
- `alpha/root-file.txt`
- `beta/prefix/test.txt`
- `beta/prefix/data/nested.txt`
- `beta/other/file.txt`

**Objects with metadata:**
- `alpha/metadata-test.bin` - Custom metadata (`x-amz-meta-author`, `x-amz-meta-project`, etc.)
- `alpha/data.json` - Content-Type: `application/json`
- `gamma/index.html` - Content-Type: `text/html` + custom metadata
- `alpha/userdata.txt` - Multiple custom metadata fields

**Server-side copies (if supported):**
- `alpha/alice-copy.bin` - Copy of `alice-object.bin`
- `beta/copied-from-alpha.txt` - Cross-bucket copy from alpha

**Large file (multipart testing):**
- `alpha/large-file.dat` (10MB)

## Use Cases

### 1. Debug Client-Specific Bugs

If you're debugging a bug that only appears with a specific client (e.g., rclone, aws-cli, mc), you can use this script to create a known-good state on your DirIO server, then test the buggy client against that state.

```bash
# Setup known state using boto3
export S3_ENDPOINT="http://localhost:8080"
export S3_ACCESS_KEY="admin"
export S3_SECRET_KEY="password"
python3 s3-generic-setup.py

# Now test the buggy client
aws s3 ls s3://alpha --endpoint-url=http://localhost:8080
```

### 2. Test DirIO Against Different S3 Implementations

```bash
# Test against real AWS S3
export S3_ENDPOINT="https://s3.us-east-1.amazonaws.com"
export S3_ACCESS_KEY="YOUR_AWS_ACCESS_KEY"
export S3_SECRET_KEY="YOUR_AWS_SECRET_KEY"
export S3_REGION="us-east-1"
python3 s3-generic-setup.py

# Test against MinIO
export S3_ENDPOINT="http://localhost:9000"
export S3_ACCESS_KEY="minioadmin"
export S3_SECRET_KEY="minioadmin"
python3 s3-generic-setup.py
```

### 3. Quickly Set Up Test Data

Instead of manually creating buckets and uploading objects, run the script to get a consistent test environment every time.

## Cleanup

**Using the MinIO client (mc):**

Remove the test alias:
```bash
mc alias rm target
```

Delete all created buckets and objects (use with caution!):
```bash
mc rb --force --dangerous target/alpha
mc rb --force --dangerous target/beta
mc rb --force --dangerous target/gamma
```

**Using boto3 (Python):**

You can use the AWS CLI with your endpoint:
```bash
aws s3 rb s3://alpha --force --endpoint-url=http://localhost:8080
aws s3 rb s3://beta --force --endpoint-url=http://localhost:8080
aws s3 rb s3://gamma --force --endpoint-url=http://localhost:8080
```

## Troubleshooting

### "mc: command not found"

Install the MinIO client as described in the Prerequisites section.

### "Failed to connect to S3 endpoint"

- Check that the endpoint URL is correct and the server is running
- Verify the access key and secret key are correct
- Try accessing the endpoint in a browser or with curl to verify it's reachable

### IAM/Policy Errors

If you see warnings about IAM users or policies failing:

- This is expected if the S3 API doesn't support IAM (e.g., DirIO might not have full IAM support yet)
- Set `SKIP_USERS=true` to skip IAM setup and only create buckets/objects
- The script will continue and create what it can

### Permission Errors

- Make sure the access key has permission to create buckets and upload objects
- Try using root/admin credentials
- Check server logs for permission errors

## Contributing

To add new test scenarios to the generic setup scripts:

1. Add the test case to both `.sh` and `.ps1` versions
2. Update this README with documentation
3. Test against multiple S3 implementations (MinIO, AWS, DirIO)
