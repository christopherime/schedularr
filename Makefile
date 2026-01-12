.PHONY: build test lint clean validate e2e-up e2e-down e2e-test e2e-clean docker-build help

# Variables
BINARY_NAME=schedularr
BUILD_DIR=./bin
MAIN_PATH=./main.go
DOCKER_IMAGE=schedularr
DOCKER_TAG=latest

# Go build flags
LDFLAGS=-ldflags="-s -w"
CGO_ENABLED=1

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build binary to ./bin/schedularr
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@CGO_ENABLED=$(CGO_ENABLED) go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Binary built: $(BUILD_DIR)/$(BINARY_NAME)"

test: ## Run tests with race detector
	@echo "Running tests..."
	@go test -race -cover ./...

lint: ## Run golangci-lint
	@echo "Running golangci-lint..."
	@golangci-lint run
	@echo "Running gosec..."
	@gosec ./...
	@echo "Running govulncheck..."
	@govulncheck ./...

clean: ## Remove build artifacts
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@rm -f $(BINARY_NAME)
	@echo "Clean complete"

validate: ## Validate all config files with CUE
	@echo "Validating configuration files..."
	@if [ -f configs/config.yaml ]; then \
		echo "Validating configs/config.yaml..."; \
		cue vet configs/config.yaml cmd/schema/config.cue; \
	fi
	@if [ -f configs/scheduler.yaml ]; then \
		echo "Validating configs/scheduler.yaml..."; \
		cue vet configs/scheduler.yaml cmd/schema/scheduler.cue; \
	fi
	@if [ -f configs/config.example.yaml ]; then \
		echo "Validating configs/config.example.yaml..."; \
		cue vet configs/config.example.yaml cmd/schema/config.cue; \
	fi
	@if [ -f configs/scheduler.example.yaml ]; then \
		echo "Validating configs/scheduler.example.yaml..."; \
		cue vet configs/scheduler.example.yaml cmd/schema/scheduler.cue; \
	fi
	@echo "Validation complete"

docker-build: ## Build Docker image
	@echo "Building Docker image $(DOCKER_IMAGE):$(DOCKER_TAG)..."
	@docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	@echo "Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)"

e2e-up: ## Start E2E test environment
	@echo "Starting E2E test environment..."
	@if [ -f e2e/docker-compose.yaml ]; then \
		cd e2e && docker-compose up -d; \
	else \
		echo "e2e/docker-compose.yaml not found"; \
		exit 1; \
	fi

e2e-down: ## Stop E2E test environment
	@echo "Stopping E2E test environment..."
	@if [ -f e2e/docker-compose.yaml ]; then \
		cd e2e && docker-compose down; \
	else \
		echo "e2e/docker-compose.yaml not found"; \
		exit 1; \
	fi

e2e-test: build ## Run E2E tests
	@echo "Running E2E tests..."
	@if [ -f e2e/test.sh ]; then \
		./e2e/test.sh; \
	else \
		echo "e2e/test.sh not found"; \
		exit 1; \
	fi

e2e-clean: ## Stop E2E environment and remove volumes
	@echo "Cleaning E2E test environment..."
	@if [ -f e2e/docker-compose.yaml ]; then \
		cd e2e && docker-compose down -v; \
	else \
		echo "e2e/docker-compose.yaml not found"; \
		exit 1; \
	fi

install: build ## Install binary to $GOPATH/bin
	@echo "Installing $(BINARY_NAME) to $(GOPATH)/bin..."
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/
	@echo "Installation complete"

fmt: ## Format Go code
	@echo "Formatting code..."
	@go fmt ./...
	@echo "Formatting complete"

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy
	@echo "Dependencies updated"
