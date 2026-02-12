# Underleaf Mycelium Spine (UMS) Makefile

.PHONY: help
help:
	@echo "Underleaf Mycelium Spine (UMS) - Available targets:"
	@echo "  proto        - Generate Go code from protobuf definitions"
	@echo "  deps         - Download and tidy Go dependencies"
	@echo "  build        - Build the UMS binary"
	@echo "  run          - Run UMS locally"
	@echo "  test         - Run all tests"
	@echo "  lint         - Run linter"
	@echo "  format       - Format code"
	@echo "  clean        - Remove build artifacts"
	@echo "  docker-build - Build Docker image"
	@echo "  docker-run   - Run Docker compose stack"
	@echo "  docker-down  - Stop Docker compose stack"

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
.PHONY: docker-build
docker-build:
	@echo "Building Docker image..."
	docker build -t underleaf/mycelium-spine:latest .
	@echo "Docker image built: underleaf/mycelium-spine:latest"

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
