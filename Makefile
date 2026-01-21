.PHONY: build clean test run docker-build docker-run

# Build the server binary
build:
	go build -o dirio-server ./cmd/server

# Build the client binary
build-client:
	go build -o dirio-client ./cmd/client

# Build both binaries
build-all: build build-client

# Clean build artifacts
clean:
	rm -f dirio-server dirio-client
	rm -rf data/

# Run tests
test:
	go test -v ./...

# Run the server locally
run:
	go run ./cmd/server --data-dir ./data --port 9000

# Build Docker image
docker-build:
	docker build -t dirio:latest .

# Run with Docker
docker-run: docker-build
	docker run -p 9000:9000 -v $(PWD)/data:/data dirio:latest

# Run with docker-compose
compose-up:
	docker-compose up -d

compose-down:
	docker-compose down

# Format code
fmt:
	go fmt ./...

# Run linter
lint:
	golangci-lint run

# Download dependencies
deps:
	go mod download
	go mod tidy
