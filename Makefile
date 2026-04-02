# Underleaf Mycelium Spine (UMS) Makefile

## help: Display this help message
.PHONY: help
help:
	@echo "Underleaf Mycelium Spine (UMS)"
	@echo "Available targets:"
	@echo ""
	@grep -E '^## ' $(firstword $(MAKEFILE_LIST)) | \
		sed -E 's/^##[[:space:]]*//' | \
		awk -F': ' '!seen[$$0]++ { printf "  %-14s %s\n", $$1, $$2 }'

## tidy-all: Run go mod tidy for all modules
.PHONY: tidy-all
tidy-all:
	@echo "Running go mod tidy for all modules..."
	@find . -name 'go.mod' -execdir go mod tidy \;
	@echo "go mod tidy complete for all modules"

## proto: Generate Go code from protobuf definitions
.PHONY: proto
proto:
	@echo "Generating Go code from protobuf definitions..."
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/ums/v1/spine.proto
	@echo "Proto code generation complete"

## help: Display this help message
.PHONY: deps
deps:
	@echo "Downloading Go dependencies..."
	go mod download
	go mod tidy
	@echo "Dependencies updated"

## build: Build the UMS binary
.PHONY: build
build: proto
	@echo "Building UMS binary..."
	go build -o bin/spine cmd/serve/main.go
	@echo "Build complete: bin/spine"

## run: Run UMS locally
.PHONY: run
run:
	@echo "Running UMS..."
	export CONFIG_PATH=${PWD}/config.yaml && \
	go run cmd/serve/main.go

## test: Run tests with race detection and coverage
.PHONY: test
test:
	@echo "Running tests..."
	go test ./... -v -race -coverprofile=coverage.out
	@echo "Tests complete"

## coverage: Generate HTML coverage report
.PHONY: coverage
coverage: test
	@echo "Generating coverage report..."
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## lint: Run linter on the codebase
.PHONY: lint
lint:
	@echo "Running linter..."
	golangci-lint run ./...
	@echo "Linting complete"

## format: Format code with gofmt and goimports
.PHONY: format
format:
	@echo "Formatting code..."
	go fmt ./...
	gofmt -s -w .
	@echo "Formatting complete"

## clean: Remove build artifacts and coverage reports
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

## docker-build: Build the Docker image (multi-arch if MULTIARCH=true)
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

## docker-push: Push the Docker image to the registry (multi-arch if MULTIARCH=true)
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

## docker-run: Start the Docker compose stack
.PHONY: docker-run
docker-run:
	@echo "Starting Docker compose stack..."
	docker-compose up -d
	@echo "Stack started. View logs: docker-compose logs -f"

## docker-down: Stop the Docker compose stack
.PHONY: docker-down
docker-down:
	@echo "Stopping Docker compose stack..."
	docker-compose down
	@echo "Stack stopped"

# ============================================================================
# SDK and CLI Targets
# ============================================================================

## sdk-build: Build the Go SDK
.PHONY: sdk-build
sdk-build:
	@echo "Building SDK..."
	cd sdk && go build ./...
	@echo "SDK build complete"

## cli-build: Build the mspinectl CLI tool
.PHONY: cli-build
cli-build:
	@echo "Building mspinectl CLI..."
	cd cmd/mspinectl && go build -o ../../bin/mspinectl .
	@echo "CLI build complete: bin/mspinectl"

## cli-install: Install mspinectl to /usr/local/bin
.PHONY: cli-install
cli-install: cli-build
	@echo "Installing mspinectl..."
	sudo cp bin/mspinectl /usr/local/bin/
	@echo "mspinectl installed to /usr/local/bin/"

# ============================================================================
# E2E Testing Targets
# ============================================================================

## e2e-build: Build the Docker images for E2E testing
.PHONY: e2e-build
e2e-build:
	@echo "Building E2E test containers..."
	docker-compose -f docker-compose.e2e.yml build
	@echo "E2E containers built"

## e2e-test: Run the E2E test suite
.PHONY: e2e-test
e2e-test:
	@echo "Running E2E test suite..."
	chmod +x e2e/test.sh
	./e2e/test.sh

## e2e-up: Start the E2E environment
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

## e2e-down: Stop the E2E environment
.PHONY: e2e-down
e2e-down:
	@echo "Stopping E2E environment..."
	docker-compose -f docker-compose.e2e.yml down

## e2e-logs: Tail logs from the E2E environment
.PHONY: e2e-logs
e2e-logs:
	docker-compose -f docker-compose.e2e.yml logs -f

## e2e-clean: Stop and remove all E2E containers, volumes, and images
.PHONY: e2e-clean
e2e-clean:
	@echo "Cleaning E2E environment..."
	docker-compose -f docker-compose.e2e.yml down -v --rmi all
	@echo "E2E environment cleaned"

## install-tools: Install development tools like protoc-gen-go and golangci-lint
.PHONY: install-tools
install-tools:
	@echo "Installing development tools..."
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "Tools installed"

## tidy-all: Run go mod tidy for all modules
.PHONY: tidy-all
tidy-all:
	@echo "Running go mod tidy for all modules..."
	@find . -name 'go.mod' -execdir go mod tidy \;
	@echo "go mod tidy complete for all modules"

# Default target
.DEFAULT_GOAL := help
