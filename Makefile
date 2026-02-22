.PHONY: build build-client build-all clean test test-unit test-integration test-coverage run docker-build docker-run release release-server release-client release-all install-templ templ-gen vendor-htmx tailwind-build tailwind-watch build-ui setup-ui

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

# Run all tests (except client tests which are designed to fail)
test:
	go test -v ./... -skip "TestAWSCLI|TestBoto3|TestMinIOMC"

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

test-clients:
	go test -v ./tests/clients/...

# Run the server locally
run:
	go run ./cmd/server serve --data-dir ./data --port 9000

# Run the server locally w/ mdns
run-mdns:
	go run ./cmd/server serve --mdns-enabled  --data-dir ./data --port 9000

run-import:
	go run ./cmd/server serve --log-level=debug --verbosity=verbose --mdns-enabled  --data-dir ./minio-data-2019 --port 9000

run-import-2022:
	go run ./cmd/server serve --log-level=debug --verbosity=verbose --mdns-enabled  --data-dir ./minio-data-2022-import --port 9000


run-mdns-debug:
	go run ./cmd/server serve --log-level=debug --verbosity=verbose --mdns-enabled  --data-dir ./data --port 9000


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

# ── UI tooling ─────────────────────────────────────────────────────────────────

# Detect OS + arch to select the correct Tailwind standalone binary.
# $(OS) is set to Windows_NT by the Windows environment (including Git Bash).
ifeq ($(OS),Windows_NT)
  TAILWIND_BIN      := ./bin/tailwindcss.exe
  TAILWIND_ARTIFACT := tailwindcss-windows-x64.exe
  TAILWIND_CHMOD    := @echo "Windows binary — no chmod needed"
  MKDIR_BIN         := if not exist bin mkdir bin
else
  UNAME_S := $(shell uname -s)
  UNAME_M := $(shell uname -m)
  TAILWIND_BIN   := ./bin/tailwindcss
  TAILWIND_CHMOD := chmod +x $(TAILWIND_BIN)
  MKDIR_BIN      := mkdir -p bin
  ifeq ($(UNAME_S),Darwin)
    ifeq ($(UNAME_M),arm64)
      TAILWIND_ARTIFACT := tailwindcss-macos-arm64
    else
      TAILWIND_ARTIFACT := tailwindcss-macos-x64
    endif
  else
    # Linux (and WSL)
    ifeq ($(UNAME_M),aarch64)
      TAILWIND_ARTIFACT := tailwindcss-linux-arm64
    else
      TAILWIND_ARTIFACT := tailwindcss-linux-x64
    endif
  endif
endif

TAILWIND_BASE_URL := https://github.com/tailwindlabs/tailwindcss/releases/latest/download

# Install the templ CLI (needed to recompile .templ → _templ.go after edits).
install-templ:
	go install github.com/a-h/templ/cmd/templ@latest

# Recompile all .templ files to _templ.go. Equivalent to: go generate ./console/...
templ-gen:
	templ generate ./console/...

# Download and vendor HTMX 2.0.4 into the embedded static directory.
vendor-htmx:
	curl -sSL https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js \
		-o console/static/js/htmx.min.js

# Download the Tailwind v4 standalone binary for the current OS/arch into ./bin/.
# The binary is gitignored; re-run this after a fresh clone or to upgrade.
download-tailwind:
	@echo "Downloading Tailwind standalone CLI ($(TAILWIND_ARTIFACT))..."
	$(MKDIR_BIN)
	curl -sSL $(TAILWIND_BASE_URL)/$(TAILWIND_ARTIFACT) -o $(TAILWIND_BIN)
	$(TAILWIND_CHMOD)
	@echo "Tailwind binary ready at $(TAILWIND_BIN)"

# Generate the minified Tailwind CSS from the templ source files.
# Requires the Tailwind standalone binary — run `make download-tailwind` first.
tailwind-build:
	$(TAILWIND_BIN) -i ./console/ui/input.css -o ./console/static/style.css --minify

# Watch templ files and regenerate CSS on change (development mode).
tailwind-watch:
	$(TAILWIND_BIN) -i ./console/ui/input.css -o ./console/static/style.css --watch

# Rebuild all generated UI artifacts (templ → Go, then Tailwind → CSS).
# Run this before `make build` whenever .templ or .css source files change.
build-ui: templ-gen tailwind-build

# First-time setup: install all UI tooling and vendor JS/CSS dependencies.
setup-ui: install-templ download-tailwind vendor-htmx build-ui
