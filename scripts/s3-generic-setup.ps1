# Generic S3 API Setup Script (PowerShell)
# -----------------------
# This script sets up test data on ANY S3-compatible API endpoint.
# It uses the MinIO client (mc) which works with any S3 API.
#
# Usage:
#   $env:S3_ENDPOINT = "http://localhost:8080"
#   $env:S3_ACCESS_KEY = "admin"
#   $env:S3_SECRET_KEY = "password123"
#   .\s3-generic-setup.ps1
#
# Optional environment variables:
#   S3_ALIAS       - mc alias name (default: "target")
#   S3_REGION      - AWS region if needed (default: "us-east-1")
#   OBJECT_SIZE    - Size of test objects in bytes (default: 65536)
#   SKIP_USERS     - Set to "true" to skip user/policy creation (default: false)
#   SKIP_POLICIES  - Set to "true" to skip bucket policies (default: false)

$ErrorActionPreference = "Stop"

# -----------------------
# Config
# -----------------------
$S3_ALIAS = if ($env:S3_ALIAS) { $env:S3_ALIAS } else { "target" }
$S3_ENDPOINT = $env:S3_ENDPOINT
$S3_ACCESS_KEY = $env:S3_ACCESS_KEY
$S3_SECRET_KEY = $env:S3_SECRET_KEY
$S3_REGION = if ($env:S3_REGION) { $env:S3_REGION } else { "us-east-1" }
$OBJECT_SIZE = if ($env:OBJECT_SIZE) { [int]$env:OBJECT_SIZE } else { 65536 }
$SKIP_USERS = if ($env:SKIP_USERS) { $env:SKIP_USERS } else { "false" }
$SKIP_POLICIES = if ($env:SKIP_POLICIES) { $env:SKIP_POLICIES } else { "false" }

# Users (for IAM creation, if supported)
$ALICE_USER = "alice"
$ALICE_PASS = "alicepass1234"
$BOB_USER = "bob"
$BOB_PASS = "bobpass1234"

# -----------------------
# Validation
# -----------------------
if (-not $S3_ENDPOINT) {
  Write-Host "❌ Error: S3_ENDPOINT is required" -ForegroundColor Red
  Write-Host ""
  Write-Host "Usage:"
  Write-Host '  $env:S3_ENDPOINT = "http://localhost:8080"'
  Write-Host '  $env:S3_ACCESS_KEY = "admin"'
  Write-Host '  $env:S3_SECRET_KEY = "password123"'
  Write-Host "  .\s3-generic-setup.ps1"
  Write-Host ""
  Write-Host "Optional variables:"
  Write-Host '  $env:S3_ALIAS = "target"        # mc alias name'
  Write-Host '  $env:S3_REGION = "us-east-1"    # AWS region'
  Write-Host '  $env:OBJECT_SIZE = 65536        # Test object size in bytes'
  Write-Host '  $env:SKIP_USERS = "true"        # Skip IAM user creation'
  Write-Host '  $env:SKIP_POLICIES = "true"     # Skip bucket policy creation'
  exit 1
}

if (-not $S3_ACCESS_KEY) {
  Write-Host "❌ Error: S3_ACCESS_KEY is required" -ForegroundColor Red
  exit 1
}

if (-not $S3_SECRET_KEY) {
  Write-Host "❌ Error: S3_SECRET_KEY is required" -ForegroundColor Red
  exit 1
}

# Check if mc is installed
$mcPath = Get-Command mc -ErrorAction SilentlyContinue
if (-not $mcPath) {
  Write-Host "❌ Error: MinIO client 'mc' is not installed" -ForegroundColor Red
  Write-Host ""
  Write-Host "Install mc:"
  Write-Host "  # Windows"
  Write-Host "  # Download from https://dl.min.io/client/mc/release/windows-amd64/mc.exe"
  Write-Host "  # Add to PATH or place in current directory"
  exit 1
}

Write-Host "✓ MinIO client (mc) found: $($mcPath.Source)" -ForegroundColor Green

# -----------------------
# Helper function to create random file
# -----------------------
function New-RandomFile {
  param([int]$Size, [string]$Path)

  $bytes = New-Object byte[] $Size
  $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
  $rng.GetBytes($bytes)
  [System.IO.File]::WriteAllBytes($Path, $bytes)
  $rng.Dispose()
}

# -----------------------
# Setup mc alias
# -----------------------
Write-Host "🔧 Configuring mc alias '$S3_ALIAS'..." -ForegroundColor Cyan
mc alias set $S3_ALIAS $S3_ENDPOINT $S3_ACCESS_KEY $S3_SECRET_KEY --api S3v4

# Test connection
Write-Host "🔌 Testing connection..." -ForegroundColor Cyan
$testResult = mc ls $S3_ALIAS 2>&1
if ($LASTEXITCODE -ne 0) {
  Write-Host "❌ Failed to connect to S3 endpoint" -ForegroundColor Red
  Write-Host "   Endpoint: $S3_ENDPOINT"
  Write-Host "   Alias: $S3_ALIAS"
  Write-Host $testResult
  exit 1
}

Write-Host "✓ Connected to $S3_ENDPOINT" -ForegroundColor Green

# -----------------------
# Create buckets
# -----------------------
Write-Host "📦 Creating test buckets..." -ForegroundColor Cyan
$buckets = @("alpha", "beta", "gamma")
foreach ($bucket in $buckets) {
  $checkResult = mc ls "$S3_ALIAS/$bucket" 2>&1
  if ($LASTEXITCODE -eq 0) {
    Write-Host "  ⚠️  Bucket '$bucket' already exists, skipping" -ForegroundColor Yellow
  } else {
    mc mb "$S3_ALIAS/$bucket" --region=$S3_REGION
    Write-Host "  ✓ Created bucket '$bucket'" -ForegroundColor Green
  }
}

# -----------------------
# Create IAM users and policies (if not skipped)
# -----------------------
if ($SKIP_USERS -ne "true") {
  Write-Host "👥 Creating IAM users and policies..." -ForegroundColor Cyan
  Write-Host "  (This may fail if the S3 API doesn't support IAM)"

  # Create policies
  $alphaPolicyJson = @"
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:*"],
      "Resource": [
        "arn:aws:s3:::alpha",
        "arn:aws:s3:::alpha/*"
      ]
    }
  ]
}
"@

  $betaPolicyJson = @"
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:*"],
      "Resource": [
        "arn:aws:s3:::beta",
        "arn:aws:s3:::beta/*"
      ]
    }
  ]
}
"@

  $tempAlphaPolicy = [System.IO.Path]::GetTempFileName()
  $tempBetaPolicy = [System.IO.Path]::GetTempFileName()
  $alphaPolicyJson | Out-File -FilePath $tempAlphaPolicy -Encoding UTF8
  $betaPolicyJson | Out-File -FilePath $tempBetaPolicy -Encoding UTF8

  # Try to create policies (may fail on non-MinIO S3 APIs)
  $result = mc admin policy add $S3_ALIAS alpha-rw $tempAlphaPolicy 2>&1
  if ($LASTEXITCODE -eq 0) {
    Write-Host "  ✓ Created policy 'alpha-rw'" -ForegroundColor Green
  } else {
    Write-Host "  ⚠️  Failed to create policy 'alpha-rw' (IAM not supported?)" -ForegroundColor Yellow
  }

  $result = mc admin policy add $S3_ALIAS beta-rw $tempBetaPolicy 2>&1
  if ($LASTEXITCODE -eq 0) {
    Write-Host "  ✓ Created policy 'beta-rw'" -ForegroundColor Green
  } else {
    Write-Host "  ⚠️  Failed to create policy 'beta-rw' (IAM not supported?)" -ForegroundColor Yellow
  }

  # Try to create users (may fail on non-MinIO S3 APIs)
  $result = mc admin user add $S3_ALIAS $ALICE_USER $ALICE_PASS 2>&1
  if ($LASTEXITCODE -eq 0) {
    Write-Host "  ✓ Created user '$ALICE_USER'" -ForegroundColor Green
    $result = mc admin policy set $S3_ALIAS alpha-rw "user=$ALICE_USER" 2>&1
    if ($LASTEXITCODE -ne 0) {
      Write-Host "  ⚠️  Failed to attach policy to '$ALICE_USER'" -ForegroundColor Yellow
    }
  } else {
    Write-Host "  ⚠️  Failed to create user '$ALICE_USER' (IAM not supported?)" -ForegroundColor Yellow
  }

  $result = mc admin user add $S3_ALIAS $BOB_USER $BOB_PASS 2>&1
  if ($LASTEXITCODE -eq 0) {
    Write-Host "  ✓ Created user '$BOB_USER'" -ForegroundColor Green
    $result = mc admin policy set $S3_ALIAS beta-rw "user=$BOB_USER" 2>&1
    if ($LASTEXITCODE -ne 0) {
      Write-Host "  ⚠️  Failed to attach policy to '$BOB_USER'" -ForegroundColor Yellow
    }
  } else {
    Write-Host "  ⚠️  Failed to create user '$BOB_USER' (IAM not supported?)" -ForegroundColor Yellow
  }

  Remove-Item $tempAlphaPolicy, $tempBetaPolicy
} else {
  Write-Host "⏭️  Skipping IAM user creation (SKIP_USERS=true)" -ForegroundColor Yellow
}

# -----------------------
# Set bucket policies (if not skipped)
# -----------------------
if ($SKIP_POLICIES -ne "true") {
  Write-Host "🌐 Setting bucket policies..." -ForegroundColor Cyan

  # Try to set public-read on gamma and beta
  $result = mc anonymous set download "$S3_ALIAS/gamma" 2>&1
  if ($LASTEXITCODE -eq 0) {
    Write-Host "  ✓ Set public-read on bucket 'gamma'" -ForegroundColor Green
  } else {
    Write-Host "  ⚠️  Failed to set public-read on 'gamma' (bucket policies not supported?)" -ForegroundColor Yellow
  }

  $result = mc anonymous set download "$S3_ALIAS/beta" 2>&1
  if ($LASTEXITCODE -eq 0) {
    Write-Host "  ✓ Set public-read on bucket 'beta'" -ForegroundColor Green
  } else {
    Write-Host "  ⚠️  Failed to set public-read on 'beta' (bucket policies not supported?)" -ForegroundColor Yellow
  }
} else {
  Write-Host "⏭️  Skipping bucket policies (SKIP_POLICIES=true)" -ForegroundColor Yellow
}

# -----------------------
# Upload basic objects
# -----------------------
Write-Host "📤 Uploading basic test objects..." -ForegroundColor Cyan

function Upload-Object {
  param([string]$Bucket, [string]$Name)

  $tmpfile = [System.IO.Path]::GetTempFileName()
  New-RandomFile -Size $OBJECT_SIZE -Path $tmpfile

  mc cp $tmpfile "$S3_ALIAS/$Bucket/$Name"
  Write-Host "  ✓ Uploaded $Bucket/$Name" -ForegroundColor Green

  Remove-Item $tmpfile
}

Upload-Object -Bucket "alpha" -Name "alice-object.bin"
Upload-Object -Bucket "beta" -Name "bob-object.bin"
Upload-Object -Bucket "gamma" -Name "public-object.bin"

# -----------------------
# Create folder structure
# -----------------------
Write-Host "📁 Creating folder structure for ListObjects testing..." -ForegroundColor Cyan
Upload-Object -Bucket "alpha" -Name "folder1/file1.txt"
Upload-Object -Bucket "alpha" -Name "folder1/file2.txt"
Upload-Object -Bucket "alpha" -Name "folder1/subfolder/deep.txt"
Upload-Object -Bucket "alpha" -Name "folder2/file1.txt"
Upload-Object -Bucket "alpha" -Name "root-file.txt"

Upload-Object -Bucket "beta" -Name "prefix/test.txt"
Upload-Object -Bucket "beta" -Name "prefix/data/nested.txt"
Upload-Object -Bucket "beta" -Name "other/file.txt"

# -----------------------
# Upload objects with metadata
# -----------------------
Write-Host "🔖 Uploading objects with various metadata..." -ForegroundColor Cyan

# Object with custom metadata
$tmpfile = [System.IO.Path]::GetTempFileName()
New-RandomFile -Size $OBJECT_SIZE -Path $tmpfile
mc cp --attr "x-amz-meta-author=TestUser;x-amz-meta-project=DirIO;x-amz-meta-version=1" `
  $tmpfile "$S3_ALIAS/alpha/metadata-test.bin"
Write-Host "  ✓ Uploaded alpha/metadata-test.bin (with custom metadata)" -ForegroundColor Green
Remove-Item $tmpfile

# Object with Content-Type
$tmpfile = [System.IO.Path]::GetTempFileName()
'{"test": "json data"}' | Out-File -FilePath $tmpfile -Encoding UTF8 -NoNewline
mc cp --attr "Content-Type=application/json" `
  $tmpfile "$S3_ALIAS/alpha/data.json"
Write-Host "  ✓ Uploaded alpha/data.json (Content-Type: application/json)" -ForegroundColor Green
Remove-Item $tmpfile

# Object with Content-Type and custom metadata
$tmpfile = [System.IO.Path]::GetTempFileName()
'<html><body>Test</body></html>' | Out-File -FilePath $tmpfile -Encoding UTF8 -NoNewline
mc cp --attr "Content-Type=text/html;x-amz-meta-page=index" `
  $tmpfile "$S3_ALIAS/gamma/index.html"
Write-Host "  ✓ Uploaded gamma/index.html (Content-Type + custom metadata)" -ForegroundColor Green
Remove-Item $tmpfile

# Object with multiple custom metadata fields
$tmpfile = [System.IO.Path]::GetTempFileName()
'user data' | Out-File -FilePath $tmpfile -Encoding UTF8 -NoNewline
mc cp --attr "x-amz-meta-user-id=12345;x-amz-meta-department=engineering;x-amz-meta-uploaded-by=alice" `
  $tmpfile "$S3_ALIAS/alpha/userdata.txt"
Write-Host "  ✓ Uploaded alpha/userdata.txt (multiple custom metadata fields)" -ForegroundColor Green
Remove-Item $tmpfile

# -----------------------
# Test CopyObject (server-side copy)
# -----------------------
Write-Host "📋 Testing CopyObject (server-side copy)..." -ForegroundColor Cyan
$result = mc cp "$S3_ALIAS/alpha/alice-object.bin" "$S3_ALIAS/alpha/alice-copy.bin" 2>&1
if ($LASTEXITCODE -eq 0) {
  Write-Host "  ✓ Server-side copy: alpha/alice-object.bin → alpha/alice-copy.bin" -ForegroundColor Green
} else {
  Write-Host "  ⚠️  Server-side copy failed (not supported?)" -ForegroundColor Yellow
}

$result = mc cp "$S3_ALIAS/alpha/folder1/file1.txt" "$S3_ALIAS/beta/copied-from-alpha.txt" 2>&1
if ($LASTEXITCODE -eq 0) {
  Write-Host "  ✓ Cross-bucket copy: alpha/folder1/file1.txt → beta/copied-from-alpha.txt" -ForegroundColor Green
} else {
  Write-Host "  ⚠️  Cross-bucket copy failed (not supported?)" -ForegroundColor Yellow
}

# -----------------------
# Upload large file (multipart)
# -----------------------
Write-Host "📦 Uploading large file for multipart testing..." -ForegroundColor Cyan
$largefile = [System.IO.Path]::GetTempFileName()
$bytes = New-Object byte[] (10 * 1024 * 1024)  # 10MB
[System.IO.File]::WriteAllBytes($largefile, $bytes)
mc cp $largefile "$S3_ALIAS/alpha/large-file.dat"
Write-Host "  ✓ Uploaded alpha/large-file.dat (10MB, likely multipart)" -ForegroundColor Green
Remove-Item $largefile

# -----------------------
# List created objects
# -----------------------
Write-Host ""
Write-Host "📋 Listing created objects..." -ForegroundColor Cyan
Write-Host ""
Write-Host "Alpha bucket:"
mc ls --recursive "$S3_ALIAS/alpha" | Select-Object -First 20
Write-Host ""
Write-Host "Beta bucket:"
mc ls --recursive "$S3_ALIAS/beta" | Select-Object -First 20
Write-Host ""
Write-Host "Gamma bucket:"
mc ls --recursive "$S3_ALIAS/gamma" | Select-Object -First 20

# -----------------------
# Done
# -----------------------
Write-Host ""
Write-Host "✅ Generic S3 setup complete!" -ForegroundColor Green
Write-Host ""
Write-Host "Endpoint: $S3_ENDPOINT"
Write-Host "Alias: $S3_ALIAS"
Write-Host "Region: $S3_REGION"
Write-Host ""
Write-Host "Buckets created:"
Write-Host "  alpha → private (alice if IAM supported)"
Write-Host "  beta  → public-read (if bucket policies supported)"
Write-Host "  gamma → public-read (if bucket policies supported)"
Write-Host ""
Write-Host "Objects created:"
Write-Host "  - Basic objects (alice-object.bin, bob-object.bin, public-object.bin)"
Write-Host "  - Folder structures (folder1/file1.txt, folder2/file1.txt, etc.)"
Write-Host "  - Objects with standard metadata (Content-Type, Content-Encoding)"
Write-Host "  - Objects with custom metadata (x-amz-meta-*)"
Write-Host "  - Server-side copies (alice-copy.bin, copied-from-alpha.txt)"
Write-Host "  - Large file for multipart testing (large-file.dat - 10MB)"
Write-Host ""
Write-Host "To test with different credentials:"
Write-Host "  mc alias set test-alice $S3_ENDPOINT alice alicepass1234"
Write-Host "  mc ls test-alice/alpha"
Write-Host ""
Write-Host "To remove the alias:"
Write-Host "  mc alias rm $S3_ALIAS"
Write-Host ""
