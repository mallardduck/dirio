# DIO Client Architecture

## Overview

`dio` is a first-party CLI client for DirIO, shipped as a separate binary (`cmd/client/`). It covers DirIO-specific operations that `mc` and the AWS CLI cannot reach — ownership management, policy simulation, service accounts — while also providing a polished experience for everyday S3 operations.

**The guiding principle:** don't duplicate what existing tools do well at parity. Justify every command by asking whether it is either (a) DirIO-specific, or (b) materially better than the equivalent `mc`/AWS CLI experience for DirIO users.

---

## Design Goals

- **DirIO-first** — the primary purpose is operations that only `dio` can do: ownership, policy simulation, service accounts, admin IAM
- **Beautiful by default** — interactive output uses Bubble Tea + Lip Gloss for color, tables, progress bars, and spinners; plain/JSON output available for scripts
- **Script-friendly** — every command supports `--output json` (or `--output plain`) to produce machine-readable output; no ANSI escape codes when stdout is not a TTY
- **Single binary, no dependencies** — no requirement to install `mc`, AWS CLI, or any other tool
- **Profile-aware** — named profiles in `~/.dirio/client.yaml` so users can switch between DirIO instances easily
- **AWS-compatible auth** — respects `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` / `AWS_ENDPOINT_URL` for drop-in compatibility

---

## Binary & Module Layout

```
dirio/
├── cmd/
│   ├── server/          # dirio server binary (existing)
│   └── client/          # dio client binary
│       └── main.go
│
└── internal/
    └── dioclient/       # client-only internals (S3 transport, profile loader, etc.)
        ├── profile/     # profile config load/save
        ├── s3/          # thin S3 client wrapper (uses aws-sdk-go-v2)
        └── render/      # output rendering (table, JSON, plain)
```

The client is a separate binary with its own `go` entry point. It may import `consoleapi/` types for DirIO-specific API shapes, but it never imports `internal/` server packages — it communicates with the server over HTTP/S3.

---

## Technology Stack

### Core CLI Framework
- **`github.com/spf13/cobra`** — command structure (consistent with the server binary)
- **`github.com/spf13/viper`** — profile/config loading from `~/.dirio/client.yaml`

### TUI & Output
| Library | Purpose |
|---|---|
| `github.com/charmbracelet/bubbletea` | Interactive TUI model (progress, confirmations, interactive pickers) |
| `github.com/charmbracelet/lipgloss` | Styling, colors, borders — already used in server's `output` package |
| `github.com/charmbracelet/bubbles` | Pre-built Bubble Tea components: spinner, progress bar, table, text input, viewport |
| `github.com/charmbracelet/glamour` | Markdown rendering for help text and rich output blocks |
| `github.com/charmbracelet/huh` | Form-style interactive prompts for `dio config init` |

### S3 Transport
- **`github.com/aws/aws-sdk-go-v2`** — standard S3 client; DirIO is S3-compatible so this works out of the box
- Custom endpoint resolver wired from the active profile's `endpoint` field

### DirIO-Specific API
- HTTP calls to the MinIO Admin API (`/minio/admin/v3/*`) for IAM operations — same endpoints `mc admin` uses
- HTTP calls to DirIO-specific console API endpoints for ownership, simulation, etc. (served at `/.dirio/`)

---

## Path Syntax — Profile-Qualified Paths

This is the most important UX concept in `dio`. It is directly inspired by `mc`'s alias system, where every remote path is prefixed with an alias name:

```
mc ls myminio/my-bucket/prefix/
```

In `dio`, the profile name plays the exact same role as an mc alias — it is the first path component and unambiguously identifies which DirIO server the path refers to:

```
dio ls local/my-bucket/prefix/
```

The general form is:

```
[profile/]bucket[/key]
```

When the profile prefix is omitted, the `default_profile` is used. This means every remote path has a well-defined "FQDN" in the form `profile/bucket/key`, with the profile as the authority component.

### Why this matters

- **Unambiguous cross-profile operations** — `dio cp local/bucket/key prod/bucket/key` copies an object between two different DirIO servers with a single command, and the intent is clear from the paths alone
- **No ambiguity between local and remote** — a bare path argument that starts with `./`, `/`, or `~` is always a local filesystem path; anything else is parsed as `[profile/]bucket[/key]`
- **Consistent with mc muscle memory** — users familiar with `mc ls alias/bucket` will find `dio ls profile/bucket` immediately intuitive

### `s3://` URI compat

`s3://bucket/key` URIs are accepted wherever a remote path is expected and resolve against the default profile. They exist for drop-in compat with scripts written for the AWS CLI — the canonical `dio` form is `profile/bucket/key`.

---

## Configuration & Profiles

Config lives at `~/.dirio/client.yaml` (separate from the server's data config). Viper loads it automatically; the `--profile` flag selects a named section.

```yaml
default_profile: local

profiles:
  local:
    endpoint: http://localhost:9000
    admin_endpoint: http://localhost:9010   # optional; falls back to endpoint
    access_key: dirio-admin
    secret_key: dirio-admin-secret
    region: us-east-1

  prod:
    endpoint: https://s3.myserver.example.com
    admin_endpoint: https://admin.myserver.example.com
    access_key: prod-admin
    secret_key: "env:PROD_SECRET_KEY"       # env var indirection
    region: us-west-2
```

**Precedence (highest to lowest):**
1. `AWS_*` environment variables (drop-in compat)
2. `DIO_*` environment variables
3. Explicit `--profile` flag
4. `default_profile` in config file
5. Built-in defaults (localhost:9000)

`dio config init` runs an interactive `huh` form that writes `~/.dirio/client.yaml`.

---

## Output Modes

Every command supports `--output` with three modes, detected automatically when not specified:

| Mode | When | Description |
|---|---|---|
| `tui` (default) | stdout is a TTY | Colored tables, spinners, progress bars via Lip Gloss + Bubbles |
| `plain` | `--output plain` or piped | Simple aligned text, no ANSI codes |
| `json` | `--output json` | Newline-delimited JSON, one object per result row |

The `render` package is the only place that knows about output mode — commands produce structured data, the renderer decides how to present it. This makes it straightforward to add `--output yaml` or `--output csv` later.

### TTY Detection
```go
// render/render.go
func DefaultMode() OutputMode {
    if !term.IsTerminal(int(os.Stdout.Fd())) {
        return ModePlain
    }
    return ModeTUI
}
```

---

## Command Structure

```
dio
├── config
│   ├── init          # Interactive profile setup (huh form)
│   ├── show          # Print active profile
│   └── profiles      # List all configured profiles
│
├── ls [[profile/]bucket[/prefix]]     # List buckets or objects
├── cp <src> <dst>                     # Upload / download / copy
├── sync <src> <dst>                   # Sync directory to/from bucket
│
├── ownership
│   ├── get [profile/]bucket[/object]  # Show current owner
│   └── transfer [profile/]bucket <user>
│
├── simulate <user> [profile/]bucket <action>   # Policy simulator
│
├── sa                         # Service accounts
│   ├── create <parent-user>
│   ├── list [user]
│   ├── info <access-key>
│   ├── update <access-key>
│   └── rm <access-key>
│
└── iam
    ├── user
    │   ├── create
    │   ├── list
    │   ├── info <access-key>
    │   ├── delete <access-key>
    │   ├── enable <access-key>
    │   └── disable <access-key>
    └── policy
        ├── create
        ├── list
        ├── info <name>
        ├── delete <name>
        ├── attach --user|--group <target> <policy>
        └── detach --user|--group <target> <policy>
```

---

## Key Command Designs

### `dio ls`

```
dio ls                          # list all buckets on the default profile
dio ls local/                   # list all buckets on the "local" profile
dio ls local/my-bucket          # list objects at root of my-bucket
dio ls local/my-bucket/logs/    # list with prefix
dio ls local/my-bucket --recursive   # flat list, no prefix grouping
dio ls local/my-bucket --output json # machine-readable
```

When the profile is omitted (`dio ls my-bucket`), the default profile is used — the same shorthand mc offers when a single alias is in active use. TUI output uses a Lip Gloss table with column widths auto-sized to the terminal width. Folders (common prefixes) are visually distinguished from objects.

### `dio cp`

```
dio cp ./file.txt local/my-bucket/path/file.txt       # upload to default or named profile
dio cp local/my-bucket/path/file.txt ./file.txt       # download
dio cp local/my-bucket/src.txt local/my-bucket/dst.txt  # copy within same server
dio cp local/my-bucket/key prod/my-bucket/key         # copy across profiles (servers)
```

The last form — cross-profile copy — is the primary advantage of the `profile/bucket/key` path format over `s3://` URIs. Large uploads show a Bubbles progress bar; multipart is used automatically above 8 MB.

### `dio sync`

```
dio sync ./local-dir local/my-bucket/prefix/
dio sync local/my-bucket/prefix/ ./local-dir
dio sync ./local-dir local/my-bucket/prefix/ --delete    # remove remote objects not in source
dio sync ./local-dir local/my-bucket/prefix/ --dry-run   # show what would happen
dio sync local/my-bucket/ prod/my-bucket/               # mirror bucket across profiles
```

`--dry-run` prints a diff-style summary (added/modified/deleted counts + file list) without making any changes.

### `dio simulate`

Calls the DirIO policy simulator endpoint and renders the result:

```
$ dio simulate alice local/my-bucket s3:GetObject
  Profile   local
  User      alice
  Bucket    my-bucket
  Action    s3:GetObject
  Result    ALLOW
  Reason    Bucket policy: statement #2 (explicit Allow, principal *)

$ dio simulate bob local/my-bucket s3:PutObject
  Profile   local
  User      bob
  Bucket    my-bucket
  Action    s3:PutObject
  Result    DENY
  Reason    Default deny — no matching Allow statement
```

With `--all-actions`, shows a full permission matrix for the user against the bucket across all common S3 actions.

### `dio config init`

An interactive `huh` form that collects endpoint, credentials, and region, then writes `~/.dirio/client.yaml`:

```
? Endpoint URL  http://localhost:9000
? Admin endpoint (leave blank to use endpoint)
? Access key    dirio-admin
? Secret key    ********
? Region        us-east-1
? Profile name  local

✓ Profile "local" saved to ~/.dirio/client.yaml
  Run "dio ls" to verify connectivity.
```

---

## TUI Component Map

| Command / Situation | Component |
|---|---|
| `cp` / `sync` file transfer in progress | `bubbles/progress` |
| Any slow API call | `bubbles/spinner` |
| `ls`, `iam user list`, `sa list` | `bubbles/table` (interactive, filterable) |
| `config init`, `iam user create` | `huh` form |
| `simulate --all-actions` | `lipgloss` styled grid |
| Long `--help` text | `glamour` markdown renderer |
| Error messages, warnings | `lipgloss` styled (red/yellow), written to stderr |

Interactive tables (`bubbles/table`) support keyboard navigation (arrow keys, `/` to filter) when stdout is a TTY — pressing Enter on a row can drill down (e.g., `ls` on a bucket row navigates into it).

---

## Error Handling & UX Conventions

- Errors are always written to **stderr**; output data to **stdout** — safe to pipe
- HTTP errors from the server surface the status code and body: `error: 403 Forbidden — bucket policy denies s3:PutObject`
- `--verbose` / `-v` prints the raw HTTP request/response for debugging
- Destructive commands (`ownership transfer`, `iam user delete`, `sa rm`) require `--confirm` or an interactive y/N prompt when stdin is a TTY
- Exit codes: `0` success, `1` usage error, `2` server/network error, `3` permission denied

---

## Implementation Phasing

The client is a new binary — all work here is additive and has no risk to the server.

### Phase 7.1 — Foundation
- `cmd/client/main.go` wired to cobra root
- `internal/dioclient/profile/` — load/save `~/.dirio/client.yaml`, profile selection, env var override
- `internal/dioclient/render/` — output mode detection, table + JSON renderers
- `dio config init` / `dio config show` / `dio config profiles`
- `dio ls` — bucket list and object list with TUI table

### Phase 7.2 — S3 Operations
- `dio cp` — upload, download, server-side copy; multipart; progress bar
- `dio sync` — directory sync with `--delete` and `--dry-run`

### Phase 7.3 — DirIO-Specific
- `dio ownership get` / `dio ownership transfer`
- `dio simulate` (single action + `--all-actions` matrix)

### Phase 7.4 — IAM & Service Accounts
- `dio sa create/list/info/update/rm`
- `dio iam user *` and `dio iam policy *`

---

## Relationship to the Server Binary

`dirio` (server) and `dio` (client) are sibling binaries in the same repo. They share:
- `internal/cli/output/` — Lip Gloss helpers (server uses these for `dirio init`, `dirio credentials set`, etc.)
- `consoleapi/` types — for DirIO-specific API response shapes

They do **not** share:
- Config loading (server reads data-dir `.dirio/config.json`; client reads `~/.dirio/client.yaml`)
- HTTP transport (client speaks outbound S3/HTTP; server speaks inbound)
- Any `internal/` server package

This separation keeps the client buildable without pulling in BoltDB, OTel, and other server-only dependencies.
