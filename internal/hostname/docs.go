// Package hostname provides best-effort, stable naming helpers intended
// specifically for configuring mDNS advertisements.
//
// This package does NOT attempt to discover or validate network identity.
// Instead, it produces deterministic, mDNS-safe name inputs that higher-level
// mDNS libraries (such as github.com/hashicorp/mdns) can probe, de-duplicate,
// and advertise according to RFC 6762.
//
// Design principles:
//
//   - Never fail: all exported functions always return a usable value.
//   - No networking: no DNS, no sockets, no interface inspection.
//   - Stable identity: names remain consistent across process restarts.
//   - mDNS-first: behavior aligns with common Bonjour / Avahi conventions.
//   - Minimal scope: collision detection and resolution are delegated to mDNS.
//
// Hostnames vs Service Instance Names:
//
// mDNS distinguishes between a host name (used for A/AAAA records) and a
// service instance name (human-facing, shown in UIs). These identifiers have
// different stability and collision expectations and are treated separately
// by this package.
//
//   - Host(): returns a stable, mDNS-safe host label (without ".local.").
//     This name is intended for A/AAAA records and should change rarely.
//
//   - Base(): returns the short, sanitized base hostname without suffixes
//     or domains. This is used as a building block for other names.
//
//   - ServiceInstance(): returns a human-friendly service instance name
//     suitable for direct use with mDNS service advertisements. Collisions
//     are expected and resolved by the mDNS implementation.
//
// Name Sources and Stability:
//
// Names are resolved in the following order:
//
//  1. Explicit override via environment variable (e.g. MYAPP_HOSTNAME)
//  2. Operating system hostname (short form, best-effort)
//  3. Application-defined fallback name
//
// To reduce collisions when multiple instances are present on the same link,
// host names include a stable per-machine suffix derived from a system
// identifier or a persisted random value.
//
// What this package intentionally does NOT do:
//
//   - Append ".local." to names
//   - Perform RFC 6762 probing or conflict resolution
//   - Inspect network interfaces or IP addresses
//   - Guarantee global uniqueness
//   - Enforce service naming policies
//
// These responsibilities belong to the mDNS implementation layer.
//
// Typical usage:
//
//	host := hostname.Host()
//	instance := hostname.ServiceInstance("My Service")
//
//	// Pass host and instance to an mDNS library for advertisement.
//
// This package is safe to use in long-running services, containers, and
// multi-instance environments where network identity may be incomplete or
// unavailable.
package hostname
