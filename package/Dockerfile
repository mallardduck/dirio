# Build context must be the project root:
#   docker build -f ./package/Dockerfile -t dirio:latest .
#
# Override the BCI version at build time:
#   docker build --build-arg BCI_VERSION=16.1 -f ./package/Dockerfile -t dirio:latest .
ARG BCI_VERSION=16.0

# ── Stage 1: Compile ──────────────────────────────────────────────────────────
FROM registry.suse.com/bci/golang:1.26 AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Download the Tailwind v4 standalone binary and regenerate all UI assets.
RUN set -eux; \
    case "$(uname -m)" in \
        aarch64|arm64) TWARCH=arm64 ;; \
        *)             TWARCH=x64   ;; \
    esac; \
    curl -fsSLo /usr/local/bin/tailwindcss \
        "https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-linux-${TWARCH}"; \
    chmod +x /usr/local/bin/tailwindcss; \
    go tool templ generate ./console/...; \
    /usr/local/bin/tailwindcss -i ./console/ui/input.css -o ./console/static/style.css --minify

ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_TIME=unknown
ARG BUILT_BY=local.docker
ARG GIT_TREE_STATE=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w \
      -X github.com/mallardduck/dirio/internal/version.Version=${VERSION} \
      -X github.com/mallardduck/dirio/internal/version.Commit=${COMMIT} \
      -X github.com/mallardduck/dirio/internal/version.BuildTime=${BUILD_TIME} \
      -X github.com/mallardduck/dirio/internal/version.BuiltBy=${BUILT_BY} \
      -X github.com/mallardduck/dirio/internal/version.GitTreeState=${GIT_TREE_STATE}" \
    -trimpath \
    -o dirio \
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

# Download gosu — used in the entrypoint to drop from root to the dirio user
# after fixing data-directory ownership on first run or image switch.
ARG GOSU_VERSION=1.17
RUN set -eux; \
    case "$(uname -m)" in \
        x86_64)  GOARCH=amd64 ;; \
        aarch64) GOARCH=arm64 ;; \
        *) echo "Unsupported architecture: $(uname -m)"; exit 1 ;; \
    esac; \
    curl -fsSLo /usr/local/bin/gosu \
        "https://github.com/tianon/gosu/releases/download/${GOSU_VERSION}/gosu-${GOARCH}"; \
    chmod +x /usr/local/bin/gosu; \
    gosu nobody true

RUN groupadd --system --gid 10001 dirio && \
    useradd  --system --uid 10001 --gid dirio \
             --no-create-home --shell /usr/sbin/nologin dirio && \
    mkdir -p /app /data && \
    chown dirio:dirio /app /data

# ── Stage 3: Minimal final image ──────────────────────────────────────────────
ARG BCI_VERSION
FROM registry.suse.com/bci/bci-micro:${BCI_VERSION}

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
COPY --from=builder /build/dirio             /app/dirio
COPY package/docker-entrypoint.sh            /app/docker-entrypoint.sh
COPY --from=rootfs /usr/local/bin/gosu       /usr/local/bin/gosu

RUN chmod +x /app/dirio /app/docker-entrypoint.sh /usr/local/bin/gosu && \
    chown 10001:10001 /app/dirio /app/docker-entrypoint.sh

EXPOSE 8080

ENV DATA_DIR=/data \
    PORT=8080 \
    DIRIO_LOG_FORMAT=json

ENTRYPOINT ["/app/docker-entrypoint.sh"]