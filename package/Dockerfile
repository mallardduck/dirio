# Build context must be the project root:
#   docker build -f ./package/Dockerfile -t dirio:latest .
#
# Override the BCI version at build time:
#   docker build --build-arg BCI_VERSION=16.1 -f ./package/Dockerfile -t dirio:latest .
ARG BCI_VERSION=16.0

# ── Stage 1: Compile ──────────────────────────────────────────────────────────
# golang:1.26 is not yet available on SUSE BCI, so we use the official image
# for compilation and bring BCI in only for the runtime layers.
FROM golang:alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

# Install code-gen tools – these layers are cached until versions change.
# templ is pinned to match go.mod so generated output is always compatible.
RUN go install github.com/a-h/templ/cmd/templ@v0.3.977

# Download the Tailwind v4 standalone binary for the current build platform.
RUN set -eux; \
    case "$(uname -m)" in \
        aarch64|arm64) TWARCH=arm64 ;; \
        *)             TWARCH=x64   ;; \
    esac; \
    mkdir -p bin; \
    wget -qO bin/tailwindcss \
        "https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-linux-${TWARCH}"; \
    chmod +x bin/tailwindcss

COPY . .

# Regenerate templ outputs and minify CSS from source so the compiled binary
# always embeds fresh assets, regardless of what was last committed.
RUN templ generate ./console/... && \
    ./bin/tailwindcss -i ./console/ui/input.css -o ./console/static/style.css --minify

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -trimpath \
    -o dirio-server \
    ./cmd/server

# ── Stage 2: Prepare chroot from SUSE BCI Base 16 ────────────────────────────
# Use the full base image to install up-to-date CA certificates via zypper,
# create a dedicated service account, and prepare the application directories
# before handing off to the minimal final layer.
ARG BCI_VERSION
FROM registry.suse.com/bci/bci-base:${BCI_VERSION} AS rootfs

RUN zypper --non-interactive refresh && \
    zypper --non-interactive install --no-recommends ca-certificates-mozilla && \
    zypper clean --all

RUN groupadd --system --gid 10001 dirio && \
    useradd  --system --uid 10001 --gid dirio \
             --no-create-home --shell /usr/sbin/nologin dirio && \
    mkdir -p /app /data && \
    chown dirio:dirio /app /data

# ── Stage 3: Minimal final image ──────────────────────────────────────────────
ARG BCI_VERSION
FROM registry.suse.com/bci/bci-minimal:${BCI_VERSION}

# Pull in up-to-date CA certificates from the prepared rootfs.
COPY --from=rootfs /etc/ssl       /etc/ssl
COPY --from=rootfs /var/lib/ca-certificates /var/lib/ca-certificates

# Add our dedicated service account.  Appending preserves the base image's
# own user/group entries so nothing in bci-minimal is disturbed.
RUN echo "dirio:x:10001:10001:DirIO service:/app:/sbin/nologin" >> /etc/passwd && \
    echo "dirio:x:10001:" >> /etc/group && \
    mkdir -p /app /data && \
    chown 10001:10001 /app /data

# Drop in the compiled server binary and the entrypoint script.
COPY --from=builder /build/dirio-server      /app/dirio-server
COPY package/docker-entrypoint.sh            /app/docker-entrypoint.sh

RUN chmod +x /app/dirio-server /app/docker-entrypoint.sh && \
    chown 10001:10001 /app/dirio-server /app/docker-entrypoint.sh

USER dirio

EXPOSE 9000

ENV DATA_DIR=/data \
    PORT=9000 \
    DIRIO_LOG_FORMAT=json

ENTRYPOINT ["/app/docker-entrypoint.sh"]