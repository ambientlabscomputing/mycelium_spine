# Underleaf Mycelium Spine (UMS) Makefile

.PHONY: help
help:
	@echo "Underleaf Mycelium Spine (UMS) - Available targets:"
	@echo ""
	@echo "Development:"
	@echo "  proto        - Generate Go code from protobuf definitions"
	@echo "  deps         - Download and tidy Go dependencies"
	@echo "  build        - Build the UMS binary"
	@echo "  run          - Run UMS locally"
	@echo "  test         - Run all tests"
	@echo "  lint         - Run linter"
	@echo "  format       - Format code"
	@echo "  clean        - Remove build artifacts"
	@echo ""
	@echo "SDK and CLI:"
	@echo "  sdk-build    - Build SDK"
	@echo "  cli-build    - Build mspinectl CLI"
	@echo "  cli-install  - Install mspinectl globally"
	@echo ""
	@echo "Docker:"
	@echo "  docker-build - Build Docker image"
	@echo "  docker-run   - Run Docker compose stack"
	@echo "  docker-down  - Stop Docker compose stack"
	@echo ""
	@echo "E2E Testing:"
	@echo "  e2e-build    - Build E2E test containers"
	@echo "  e2e-test     - Run E2E test suite"
	@echo "  e2e-up       - Start E2E environment"
	@echo "  e2e-down     - Stop E2E environment"
	@echo "  e2e-logs     - View E2E logs"
	@echo "  e2e-clean    - Clean E2E volumes and images"

# Proto generation
.PHONY: proto
proto:
	@echo "Generating Go code from protobuf definitions..."
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/ums/v1/spine.proto
	@echo "Proto code generation complete"

# Dependencies
.PHONY: deps
deps:
	@echo "Downloading Go dependencies..."
	go mod download
	go mod tidy
	@echo "Dependencies updated"

# Build
.PHONY: build
build: proto
	@echo "Building UMS binary..."
	go build -o bin/spine cmd/serve/main.go
	@echo "Build complete: bin/spine"

# Run
.PHONY: run
run:
	@echo "Running UMS..."
	export CONFIG_PATH=${PWD}/config.yaml && \
	go run cmd/serve/main.go

# Test
.PHONY: test
test:
	@echo "Running tests..."
	go test ./... -v -race -coverprofile=coverage.out
	@echo "Tests complete"

# Test coverage
.PHONY: coverage
coverage: test
	@echo "Generating coverage report..."
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Lint
.PHONY: lint
lint:
	@echo "Running linter..."
	golangci-lint run ./...
	@echo "Linting complete"

# Format
.PHONY: format
format:
	@echo "Formatting code..."
	go fmt ./...
	gofmt -s -w .
	@echo "Formatting complete"

# Clean
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -f coverage.out coverage.html
	rm -rf proto/ums/v1/*.pb.go
	@echo "Clean complete"

# Docker
#
# Set MULTIARCH=true to build for linux/amd64 + linux/arm64 via buildx.
# Example: make docker-push MULTIARCH=true
MULTIARCH ?= false
BUILDX_BUILDER ?= underleaf-builder

.PHONY: _ensure-builder
_ensure-builder:
	@if [ "$(MULTIARCH)" = "true" ]; then \
		if ! docker buildx inspect $(BUILDX_BUILDER) > /dev/null 2>&1; then \
			echo "  → creating buildx builder '$(BUILDX_BUILDER)' (docker-container driver)..."; \
			docker buildx create --name $(BUILDX_BUILDER) --driver docker-container --use; \
		else \
			docker buildx use $(BUILDX_BUILDER); \
		fi; \
	fi

.PHONY: docker-build
docker-build: _ensure-builder
	@echo "Building Docker image..."
	@if [ "$(MULTIARCH)" = "true" ]; then \
		echo "  → multi-arch build (linux/amd64, linux/arm64)"; \
		docker buildx build \
			--platform linux/amd64,linux/arm64 \
			-t ambientlabsjose/mycelium_spine:develop \
			--load=false \
			.; \
	else \
		docker build -t ambientlabsjose/mycelium_spine:develop .; \
	fi
	@echo "Docker image built: ambientlabsjose/mycelium_spine:develop"

.PHONY: docker-push
docker-push: _ensure-builder
	@echo "Pushing Docker image to registry..."
	@if [ "$(MULTIARCH)" = "true" ]; then \
		echo "  → multi-arch push (linux/amd64, linux/arm64)"; \
		docker buildx build \
			--platform linux/amd64,linux/arm64 \
			-t ambientlabsjose/mycelium_spine:develop \
			--push \
			.; \
	else \
		docker build -t ambientlabsjose/mycelium_spine:develop . && \
		docker push ambientlabsjose/mycelium_spine:develop; \
	fi
	@echo "Docker image pushed: ambientlabsjose/mycelium_spine:develop"

.PHONY: docker-run
docker-run:
	@echo "Starting Docker compose stack..."
	docker-compose up -d
	@echo "Stack started. View logs: docker-compose logs -f"

.PHONY: docker-down
docker-down:
	@echo "Stopping Docker compose stack..."
	docker-compose down
	@echo "Stack stopped"

# ============================================================================
# SDK and CLI Targets
# ============================================================================

# Build SDK
.PHONY: sdk-build
sdk-build:
	@echo "Building SDK..."
	cd sdk && go build ./...
	@echo "SDK build complete"

# Build CLI
.PHONY: cli-build
cli-build:
	@echo "Building mspinectl CLI..."
	cd cmd/mspinectl && go build -o ../../bin/mspinectl .
	@echo "CLI build complete: bin/mspinectl"

# Install CLI globally
.PHONY: cli-install
cli-install: cli-build
	@echo "Installing mspinectl..."
	sudo cp bin/mspinectl /usr/local/bin/
	@echo "mspinectl installed to /usr/local/bin/"

# ============================================================================
# E2E Testing Targets
# ============================================================================

# Build E2E containers
.PHONY: e2e-build
e2e-build:
	@echo "Building E2E test containers..."
	docker-compose -f docker-compose.e2e.yml build
	@echo "E2E containers built"

# Run E2E test suite
.PHONY: e2e-test
e2e-test:
	@echo "Running E2E test suite..."
	chmod +x e2e/test.sh
	./e2e/test.sh

# Start E2E environment
.PHONY: e2e-up
e2e-up:
	@echo "Starting E2E environment..."
	docker-compose -f docker-compose.e2e.yml up -d
	@echo "Waiting for services..."
	@sleep 15
	@echo ""
	@echo "✓ E2E environment is ready!"
	@echo ""
	@echo "Example commands:"
	@echo "  make e2e-logs"
	@echo "  make e2e-test"
	@echo "  docker-compose -f docker-compose.e2e.yml exec test-client-1 mspinectl subscribe --target-type server --target-id test-server-01 --auto-ack"

# Stop E2E environment
.PHONY: e2e-down
e2e-down:
	@echo "Stopping E2E environment..."
	docker-compose -f docker-compose.e2e.yml down

# View E2E logs
.PHONY: e2e-logs
e2e-logs:
	docker-compose -f docker-compose.e2e.yml logs -f

# Clean E2E environment
.PHONY: e2e-clean
e2e-clean:
	@echo "Cleaning E2E environment..."
	docker-compose -f docker-compose.e2e.yml down -v --rmi all
	@echo "E2E environment cleaned"

# Install tools
.PHONY: install-tools
install-tools:
	@echo "Installing development tools..."
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "Tools installed"

# Default target
.DEFAULT_GOAL := help
