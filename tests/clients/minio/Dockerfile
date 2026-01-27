FROM alpine:3.19

# Install dependencies
RUN apk add --no-cache \
    ca-certificates \
    curl \
    bash

# Install MinIO mc client
RUN curl -fsSL https://dl.min.io/client/mc/release/linux-amd64/mc \
    -o /usr/local/bin/mc \
    && chmod +x /usr/local/bin/mc

# Set working directory
WORKDIR /work

# Default shell
ENTRYPOINT ["/bin/sh"]