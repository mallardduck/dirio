.PHONY: build build-client build-all clean test test-unit test-integration test-coverage run docker-build docker-run release release-server release-client release-all

# Build the server binary using GoReleaser (local snapshot)
build:
	goreleaser build --id=server --snapshot --clean --single-target

# Build the client binary using GoReleaser (local snapshot)
build-client:
	goreleaser build --id=client --snapshot --clean --single-target

# Build both binaries using GoReleaser (local snapshot)
build-all:
	goreleaser build --snapshot --clean --single-target

# Build for all platforms (server only)
release-server:
	goreleaser build --id=server --snapshot --clean

# Build for all platforms (client only)
release-client:
	goreleaser build --id=client --snapshot --clean

# Build for all platforms (both)
release-all:
	goreleaser build --snapshot --clean

# Clean build artifacts
clean:
	rm -f dirio dirio-client
	rm -rf data/ dist/

# Run all tests
test:
	go test -v ./...

# Run unit tests only (excludes integration tests)
test-unit:
	go test -v ./internal/... ./pkg/...

# Run integration tests only
test-integration:
	go test -v ./tests/integration/...

# Run tests with coverage
test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run the server locally
run:
	go run ./cmd/server serve --data-dir ./data --port 9000

# Run the server locally w/ mdns
run-mdns:
	go run ./cmd/server serve --log-level=debug --verbosity=verbose --mdns-enabled  --data-dir ./data --port 9000

run-mdns-master:
	go run ./cmd/server serve --log-level=debug --verbosity=verbose --mdns-enabled --mdns-mode=master --data-dir ./data --port 9000


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
