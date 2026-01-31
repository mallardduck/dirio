# jsonutil

Unified JSON encoding package that automatically formats JSON output based on environment mode.

## Features

- **Automatic formatting**: Compact JSON in production, pretty-printed in debug mode
- **Environment-based**: Controlled via `DIRIO_DEBUG` environment variable
- **Drop-in replacement**: Simple API compatible with standard `encoding/json`
- **Filesystem integration**: Built-in support for billy.Filesystem

## Usage

### Basic Encoding

```go
import "github.com/mallardduck/dirio/internal/jsonutil"

data := MyStruct{Name: "example", Count: 42}

// Automatically formats based on DIRIO_DEBUG
bytes, err := jsonutil.Marshal(data)
```

### Encoding to File

```go
import "github.com/mallardduck/dirio/internal/jsonutil"

data := MyStruct{Name: "example", Count: 42}

// Writes JSON to file, auto-formatted based on environment
err := jsonutil.MarshalToFile(fs, "config.json", data)
```

### Decoding

```go
import "github.com/mallardduck/dirio/internal/jsonutil"

var data MyStruct
err := jsonutil.Unmarshal(bytes, &data)
```

## Formatting Control

The package automatically selects JSON formatting based on multiple sources (checked in order):

### 1. Environment Variable Override

| DIRIO_DEBUG | Output Format |
|-------------|---------------|
| `1` or `true` | Pretty-printed (indented with 2 spaces) |
| `0`, `false`, or unset | Check config flags |

### 2. Application Config Flags

The following flag combinations enable pretty-printed JSON:

| Flags | Pretty JSON |
|-------|-------------|
| `--debug` | Yes |
| `--log-level=debug --verbosity=verbose` | Yes (both required) |
| Any other combination | No (compact) |

### Examples

**Production mode (default):**
```bash
./dirio
```
Output: `{"name":"example","count":42}`

**Debug mode - via flag:**
```bash
./dirio --debug
```
Output:
```json
{
  "name": "example",
  "count": 42
}
```

**Debug mode - via log flags:**
```bash
./dirio --log-level=debug --verbosity=verbose
```
Output:
```json
{
  "name": "example",
  "count": 42
}
```

**Debug mode - via environment:**
```bash
export DIRIO_DEBUG=1
./dirio
```
Output:
```json
{
  "name": "example",
  "count": 42
}
```

## Migration Guide

### Before

```go
import "encoding/json"

// Inconsistent formatting across codebase
data1, err := json.MarshalIndent(user, "", "  ")  // User metadata
data2, err := json.Marshal(bucket)                 // Bucket metadata
data3, err := json.MarshalIndent(config, "", "  ") // Data config
```

### After

```go
import "github.com/mallardduck/dirio/internal/jsonutil"

// Consistent API, automatic formatting
data1, err := jsonutil.Marshal(user)    // Auto-formatted
data2, err := jsonutil.Marshal(bucket)  // Auto-formatted
data3, err := jsonutil.Marshal(config)  // Auto-formatted
```

### File Writing - Before

```go
import (
    "encoding/json"
    "github.com/mallardduck/dirio/internal/util"
)

data, err := json.Marshal(meta)
if err != nil {
    return err
}
return util.WriteFile(m.metadataFS, metaPath, data, 0644)
```

### File Writing - After

```go
import "github.com/mallardduck/dirio/internal/jsonutil"

return jsonutil.MarshalToFile(m.metadataFS, metaPath, meta)
```

## Benefits

1. **Consistency**: All JSON output follows the same formatting rules
2. **Simplicity**: One function call handles both encoding and formatting
3. **Production-ready**: Compact output reduces file sizes in production
4. **Developer-friendly**: Pretty-printed output makes debugging easier
5. **No code changes**: Switch between modes via environment variable only

## Testing

Run the test suite:

```bash
go test ./internal/jsonutil/...
```

Test with debug mode:

```bash
DIRIO_DEBUG=1 go test ./internal/jsonutil/... -v
```
