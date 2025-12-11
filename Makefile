.PHONY: build clean test run docker-build docker-run install fmt lint

# Variables
BINARY_NAME=hypasis-sync
DOCKER_IMAGE=hypasis/sync-protocol
VERSION?=latest

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BINARY_NAME) ./cmd/hypasis-sync

# Build for multiple platforms
build-all:
	@echo "Building for multiple platforms..."
	GOOS=linux GOARCH=amd64 go build -o $(BINARY_NAME)-linux-amd64 ./cmd/hypasis-sync
	GOOS=darwin GOARCH=amd64 go build -o $(BINARY_NAME)-darwin-amd64 ./cmd/hypasis-sync
	GOOS=darwin GOARCH=arm64 go build -o $(BINARY_NAME)-darwin-arm64 ./cmd/hypasis-sync

# Install binary
install:
	@echo "Installing $(BINARY_NAME)..."
	go install ./cmd/hypasis-sync

# Run the application
run:
	@echo "Running $(BINARY_NAME)..."
	go run ./cmd/hypasis-sync --data-dir=./data

# Run with example config
run-config:
	@echo "Running with example config..."
	cp config.example.yaml config.yaml
	go run ./cmd/hypasis-sync --config=config.yaml

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Lint code
lint:
	@echo "Linting code..."
	golangci-lint run ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME)-*
	rm -rf ./data
	rm -f coverage.out coverage.html

# Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(VERSION) .
	docker tag $(DOCKER_IMAGE):$(VERSION) $(DOCKER_IMAGE):latest

# Run Docker container
docker-run:
	@echo "Running Docker container..."
	docker run -d \
		--name hypasis-sync \
		-p 8080:8080 \
		-p 9090:9090 \
		-p 30303:30303 \
		-v $(PWD)/data:/data/hypasis \
		$(DOCKER_IMAGE):$(VERSION)

# Stop Docker container
docker-stop:
	@echo "Stopping Docker container..."
	docker stop hypasis-sync || true
	docker rm hypasis-sync || true

# Docker Compose up
compose-up:
	@echo "Starting with Docker Compose..."
	cd deployments/docker && docker-compose up -d

# Docker Compose down
compose-down:
	@echo "Stopping Docker Compose..."
	cd deployments/docker && docker-compose down

# Deploy to Kubernetes
k8s-deploy:
	@echo "Deploying to Kubernetes..."
	kubectl apply -f deployments/kubernetes/hypasis-sync.yaml

# Remove from Kubernetes
k8s-delete:
	@echo "Removing from Kubernetes..."
	kubectl delete -f deployments/kubernetes/hypasis-sync.yaml

# Tidy dependencies
deps:
	@echo "Tidying dependencies..."
	go mod tidy

# Download dependencies
deps-download:
	@echo "Downloading dependencies..."
	go mod download

# Help
help:
	@echo "Hypasis Sync Protocol - Makefile targets:"
	@echo ""
	@echo "  build          - Build the binary"
	@echo "  build-all      - Build for multiple platforms"
	@echo "  install        - Install the binary"
	@echo "  run            - Run the application"
	@echo "  run-config     - Run with example config"
	@echo "  test           - Run tests"
	@echo "  test-coverage  - Run tests with coverage"
	@echo "  fmt            - Format code"
	@echo "  lint           - Lint code"
	@echo "  clean          - Clean build artifacts"
	@echo ""
	@echo "Docker:"
	@echo "  docker-build   - Build Docker image"
	@echo "  docker-run     - Run Docker container"
	@echo "  docker-stop    - Stop Docker container"
	@echo "  compose-up     - Start with Docker Compose"
	@echo "  compose-down   - Stop Docker Compose"
	@echo ""
	@echo "Kubernetes:"
	@echo "  k8s-deploy     - Deploy to Kubernetes"
	@echo "  k8s-delete     - Remove from Kubernetes"
	@echo ""
	@echo "Dependencies:"
	@echo "  deps           - Tidy dependencies"
	@echo "  deps-download  - Download dependencies"
