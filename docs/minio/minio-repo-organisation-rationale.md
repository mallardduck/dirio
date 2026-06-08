# MinIO Repo Organisation Rationale

> Why MinIO split what could have been one monorepo into ~25 separate Go modules,
> and what structural decisions that forced on the codebase.

---

## The core problem: shared types without shared implementations

The fundamental tension in any S3 server project is that three distinct programs —
a server, a CLI client, and a web UI — all need to speak the same language. They share
type definitions (what does a `BucketInfo` look like?), they share wire formats (how is
an admin request serialised?), and they share low-level utilities (how do you load a
TLS certificate?).

In a monorepo, sharing is easy but comes with a hidden cost: nothing stops a developer
from importing a server-internal type directly into the console code. Over time, this
creates an invisible web of coupling. You can't change the server without potentially
breaking the console; you can't release the SDK without releasing the server too.

MinIO's multi-repo strategy is the answer to that problem.

---

## The three-tier rule

Every dependency arrow in the MinIO graph points *downward* through exactly three tiers
and never upward or sideways between binaries at the same tier.

```
Tier 1 (binaries)  →  Tier 2 (SDKs)  →  Tier 3 (primitives)
```

**Tier 1** packages (`minio`, `mc`, `console`) are the user-facing programs. They may
import anything from Tier 2 or Tier 3. They must never import each other at the same
tier — except for the one deliberate exception where `console` imports `mc` to reuse
its file-transfer logic, and `minio` imports `console` to embed the web UI. Both are
one-directional imports into a lower-level package; neither creates a cycle.

**Tier 2** packages (`minio-go`, `madmin-go`, `pkg`) are the stable public interfaces.
They define the contracts that Tier 1 programs depend on. They are versioned
independently with full semver so consumers can pin and upgrade them separately from
any binary release. They may import Tier 3 primitives but never Tier 1 binaries.

**Tier 3** packages (`highwayhash`, `sio`, `mux`, etc.) do exactly one low-level thing
and have zero `minio/*` dependencies. They can be imported by anyone without pulling
in any S3 logic.

---

## Reason 1: preventing circular dependencies at the Go module level

Go's module system forbids import cycles within a module but cannot enforce anything
across module boundaries. The only way to prevent Tier 1 from importing each other
in a problematic cycle is to put them in separate modules. If `mc` and `minio/minio`
were in the same module, nothing in the toolchain would stop `minio/minio` from
importing `mc`'s internal packages, which might then import back into `minio/minio`.

By putting each binary in its own module:
- `mc` cannot import `minio/minio` without adding it as an explicit `go.mod` dependency.
- That dependency would be visible, reviewable, and obviously wrong.
- The structure makes the correct architecture the path of least resistance.

---

## Reason 2: independent release cadences

The S3 client SDK (`minio-go`) needs to be released whenever AWS adds a new API feature
or when a bug is discovered by a third-party user. Tying that release to the server's
release cycle would mean:
- A bug in the SDK blocks a server release until the server is also ready.
- A server release forces an SDK version bump even when nothing in the SDK changed.
- External users of the SDK (not using the MinIO server at all) are held hostage to
  the server's internal development schedule.

Separate modules with their own `go.mod` files mean `minio-go` can ship a patch release
in an afternoon without touching the server. The server then updates its `go.mod`
dependency on its own schedule.

---

## Reason 3: madmin-go solves the shared-types problem cleanly

Both the server (which *implements* the admin API) and `mc`/`console` (which *call* it)
need the same request/response struct definitions, error codes, and HTTP route constants.

If those types lived in `minio/minio`, importing them from `mc` or `console` would drag
in the entire server dependency tree. Instead, `madmin-go` provides a lightweight,
importable package that contains *only* the interface — types, HTTP paths, and client
methods — with no server implementation. The server depends on `madmin-go` for the
types; the server's handler implementations live in `minio/minio` itself.

This pattern (extract shared types into a neutral module) is the standard Go remedy for
the problem of "many packages need to speak the same language but shouldn't know about
each other's implementations."

---

## Reason 4: the console embed pattern requires a library, not a command

If the console were structured as a standalone `main` package inside `minio/minio`, it
could not be imported as a library. The server would have to spawn a subprocess,
coordinate ports, and manage two processes. By making `minio/console` a proper Go
module with exported functions, the server can do:

```go
import "github.com/minio/console"

func main() {
    // ...
    console.RegisterHandlers(router)
    // ...
}
```

The console's compiled React assets are embedded via `go:embed` in the console package,
so the server binary picks them up at compile time. The result is one binary, one
process, one port for S3, one for the console.

This pattern **requires** separate modules. A command package (`main`) is not importable.
Only by extracting the console into its own `github.com/minio/console` module with
non-`main` exported packages can the server import and embed it.

---

## Reason 5: Tier 3 primitives must be importable without S3 baggage

If `minio/highwayhash` (a fast hashing library) lived inside `minio/minio`, any project
that wanted to use it would need to declare a dependency on the entire MinIO server —
including its 239-import dependency tree covering cloud provider SDKs, message queues,
and database drivers. Nobody would use it.

By extracting primitives into their own minimal modules, they become useful to the
broader Go community and accumulate their own user base (`highwayhash` has 382
importers). This also means MinIO benefits from community bug reports and contributions
on the primitives without exposing the server's internals.

---

## Reason 6: separate modules enable separate licensing strategy

The server and most Tier 2/3 packages are AGPL-3.0. But `minio-go`, `highwayhash`,
`sio`, `mux`, and most primitives are Apache-2.0. The Apache license is permissive —
no network-service copyleft obligation. AGPL-3.0 requires you to open-source any
derivative service that uses the library over a network.

A monorepo would struggle to maintain this split licensing cleanly. Separate modules
make it unambiguous: if you import `minio/minio-go/v7` (Apache), you have no AGPL
obligation. If you import `minio/madmin-go` (AGPL), your service must open-source
modifications.

---

## What this means for building a replacement

If you are building a replacement for `minio/minio`, this architecture tells you exactly
what surface area you need to be compatible with:

1. **The S3 API** — what `minio/minio-go/v7` calls. This is the Amazon S3 API; any
   conforming S3 implementation works.
2. **The admin API** — what `minio/madmin-go/v3` calls. This is MinIO-specific; if you
   want `mc admin` commands to work against your server, you need to implement these
   endpoints.
3. **The console API** — what the embedded console frontend calls. This is the REST API
   generated from the console's swagger spec.

You do not need to use any of the `minio/*` primitive packages in your implementation.
They are the *how* of MinIO's implementation, not the *what* of the interface. You can
substitute your own encryption, routing, or hashing code entirely.

The module separation is what makes this clear: the interface is defined in Tier 2
modules that will continue to be maintained (they serve AIStor as well as the community
edition). The implementation is in the now-archived Tier 1 server that you are replacing.

---

## Summary table

| Design decision | Problem it solves |
|-----------------|------------------|
| Separate module per binary | Prevents accidental cross-binary coupling; enforces explicit dependency declarations |
| `madmin-go` as neutral types module | Lets server and clients share types without circular dependency |
| `console` as importable library | Enables server to embed web UI in a single binary via `go:embed` |
| Tier 3 primitives as standalone modules | Makes utilities reusable without dragging in S3 baggage |
| Separate release cycles | SDK can ship patches without a full server release |
| Split Apache-2.0 / AGPL-3.0 | Permissive licensing for widely-used client SDKs; copyleft for server components |
