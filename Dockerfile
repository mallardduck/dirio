FROM golang:1.26-alpine AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build server binary
RUN CGO_ENABLED=0 GOOS=linux go build -o dirio-server ./cmd/server

# Final image
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/dirio-server .

# Create data directory
RUN mkdir -p /data

# Expose S3 port
EXPOSE 9000

# Set default environment variables
ENV DATA_DIR=/data
ENV PORT=9000
ENV ACCESS_KEY=minioadmin
ENV SECRET_KEY=minioadmin

# Run server
ENTRYPOINT ["/app/dirio-server"]
CMD ["--data-dir", "/data", "--port", "9000"]
