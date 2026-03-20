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

# ── Stage 2: Seed image for the chroot ───────────────────────────────────────
# Captured as a named stage so the zypper stage can COPY its filesystem into
# /chroot and then install packages on top with --installroot.
ARG BCI_VERSION
FROM registry.suse.com/bci/bci-micro:${BCI_VERSION} AS micro

# ── Stage 3: Build runtime filesystem via zypper --installroot ────────────────
# bci-base provides zypper; we seed /chroot from bci-micro then install
# everything we need into it.  zypper resolves all transitive dependencies
# automatically, so no manual shared-library COPY lines are required.
ARG BCI_VERSION
FROM registry.suse.com/bci/bci-base:${BCI_VERSION} AS zypper

RUN mkdir /chroot
COPY --from=micro / /chroot/

RUN zypper --non-interactive refresh && \
    zypper --non-interactive install --no-recommends libcap-progs && \
    zypper --installroot /chroot --non-interactive install --no-recommends \
        ca-certificates-mozilla \
        bind-utils \
        grep \
        gawk \
        tcpdump \
        curl \
        iproute2 \
        netcat \
        strace \
        lsof \
        procps \
        vim && \
    zypper --installroot /chroot clean -a && \
    rm -rf /chroot/var/cache/zypp/* /chroot/var/log/zypp/* /chroot/etc/zypp/

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
    gosu nobody true; \
    cp /usr/local/bin/gosu /chroot/usr/local/bin/gosu

# Add the dedicated service account and application directories.
RUN echo "dirio:x:10001:10001:DirIO service:/app:/sbin/nologin" >> /chroot/etc/passwd && \
    echo "dirio:x:10001:" >> /chroot/etc/group && \
    mkdir -p /chroot/app /chroot/data && \
    chown 10001:10001 /chroot/app /chroot/data

# Drop in the compiled server binary and the entrypoint script.
COPY --from=builder /build/dirio          /chroot/app/dirio
COPY package/docker-entrypoint.sh         /chroot/app/docker-entrypoint.sh
RUN chmod +x /chroot/app/dirio /chroot/app/docker-entrypoint.sh /chroot/usr/local/bin/gosu && \
    chown 10001:10001 /chroot/app/dirio /chroot/app/docker-entrypoint.sh && \
    setcap cap_net_bind_service=+eip /chroot/app/dirio

# ── Stage 4: Scratch final image ──────────────────────────────────────────────
# Copy the prepared filesystem wholesale — no base image means no inherited
# labels, only what we explicitly declare here.
FROM scratch

COPY --from=zypper /chroot /

LABEL org.opencontainers.image.vendor="MallardDuck" \
      org.opencontainers.image.description="Filesystem-native S3-compatible object store. Objects are files, buckets are directories."

EXPOSE 8080
EXPOSE 5353/udp

ENV PATH=/app:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin \
    DATA_DIR=/data \
    PORT=8080 \
    DIRIO_LOG_FORMAT=json

ENTRYPOINT ["/app/docker-entrypoint.sh"]