#!/bin/sh
# DirIO container entrypoint.
#
# NOTE: Viper also reads DIRIO_* environment variables automatically
# (e.g. DIRIO_DATA_DIR, DIRIO_PORT, DIRIO_ACCESS_KEY).  The explicit
# flag-building below is kept for clarity and to ensure sane defaults when
# those variables are unset.
set -e

# ── Root: fix data-directory ownership then re-exec as dirio ─────────────────
# When started as root (the default when no --user is supplied), chown the data
# directory to the dirio service account and drop privileges via gosu.  This
# makes the container tolerant of volumes previously owned by any other image
# (e.g. minio/minio which writes as root).
#
# When started as a non-root user (via --user / compose user:) this block is
# skipped entirely and the server runs as whatever user was specified.
if [ "$(id -u)" = "0" ]; then
    chown -R 10001:10001 "${DATA_DIR:-/data}"
    exec gosu dirio "$0" "$@"
fi

# ── Pass-through: subcommand or absolute path ─────────────────────────────────
# If the caller supplies a known dirio subcommand, or an absolute path (e.g.
# /bin/sh for debug), exec it directly without modification.
case "${1:-}" in
    serve | version | credentials | key | routes | init | help | completion)
        exec /app/dirio-server "$@"
        ;;
    /*)
        exec "$@"
        ;;
esac

# ── Auto-configure the 'serve' subcommand from environment variables ──────────
set -- serve \
    --data-dir "${DATA_DIR:-/data}" \
    --port     "${PORT:-9000}"

[ -n "${ACCESS_KEY:-}"       ] && set -- "$@" --access-key       "$ACCESS_KEY"
[ -n "${SECRET_KEY:-}"       ] && set -- "$@" --secret-key       "$SECRET_KEY"

[ -n "${LOG_LEVEL:-}"        ] && set -- "$@" --log-level        "$LOG_LEVEL"
[ -n "${LOG_FORMAT:-}"       ] && set -- "$@" --log-format       "$LOG_FORMAT"
[ -n "${VERBOSITY:-}"        ] && set -- "$@" --verbosity        "$VERBOSITY"
[ "${DEBUG:-false}" = "true" ] && set -- "$@" --debug

[ "${MDNS_ENABLED:-false}" = "true" ] && set -- "$@" --mdns-enabled
[ -n "${MDNS_NAME:-}"        ] && set -- "$@" --mdns-name        "$MDNS_NAME"
[ -n "${MDNS_HOSTNAME:-}"    ] && set -- "$@" --mdns-hostname    "$MDNS_HOSTNAME"
[ -n "${MDNS_MODE:-}"        ] && set -- "$@" --mdns-mode        "$MDNS_MODE"
[ -n "${CANONICAL_DOMAIN:-}" ] && set -- "$@" --canonical-domain "$CANONICAL_DOMAIN"

[ "${CONSOLE_ENABLED:-true}" = "false" ] && set -- "$@" --console=false
[ -n "${CONSOLE_PORT:-}"     ] && set -- "$@" --console-port     "$CONSOLE_PORT"

[ -n "${SHUTDOWN_TIMEOUT:-}" ] && set -- "$@" --shutdown-timeout "$SHUTDOWN_TIMEOUT"

exec /app/dirio-server "$@"