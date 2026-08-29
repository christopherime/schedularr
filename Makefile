.PHONY: build test lint clean validate e2e-up e2e-down e2e-test e2e-clean docker-build generate help web-presence web-types web-check web-build web

# Variables
BINARY_NAME=schedularr
BUILD_DIR=./bin
MAIN_PATH=./main.go
DOCKER_IMAGE=schedularr
DOCKER_TAG=latest
HUGO_MIN_VERSION=0.120

# Go build flags
LDFLAGS=-ldflags="-s -w"
CGO_ENABLED=1

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

web-presence: ## Verify web/public is non-empty (placeholder or hugo output) so go:embed builds
	@if [ ! -d web/public ] || [ -z "$$(ls -A web/public 2>/dev/null)" ]; then \
		echo "web/public is empty -- run 'make web' (or 'make web-build'), or restore the committed placeholder at web/public/index.html"; \
		exit 1; \
	fi

build: web-presence ## Build binary to ./bin/schedularr
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@CGO_ENABLED=$(CGO_ENABLED) go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Binary built: $(BUILD_DIR)/$(BINARY_NAME)"

test: ## Run tests with race detector
	@echo "Running tests..."
	@go test -race -cover ./...

lint: ## Run golangci-lint, gosec, govulncheck, and web-check -- runs every step even if an earlier one fails, and reports all failures at the end
	@status=0; \
	echo "Running golangci-lint..."; \
	golangci-lint run || status=1; \
	echo "Running gosec..."; \
	gosec ./... || status=1; \
	echo "Running govulncheck..."; \
	govulncheck ./... || status=1; \
	if command -v npm >/dev/null 2>&1; then \
		$(MAKE) web-check || status=1; \
	else \
		echo "npm not found -- skipping web-check"; \
	fi; \
	if [ $$status -ne 0 ]; then \
		echo "lint: one or more steps failed (see above)"; \
	else \
		echo "lint: all steps passed"; \
	fi; \
	exit $$status

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

generate: ## Regenerate OpenAPI server code
	@echo "Regenerating OpenAPI server code..."
	@go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml
	@echo "Codegen complete"

web-types: ## Regenerate web/assets/ts/gen/types.d.ts from api/openapi.yaml
	@echo "Generating TypeScript types from api/openapi.yaml..."
	@npm run types --prefix web

web-check: web-types ## Type-check the web TS sources (skips with a notice if node is absent)
	@if command -v npm >/dev/null 2>&1; then \
		echo "Type-checking web/assets/ts..."; \
		npm run check --prefix web; \
	else \
		echo "npm not found -- skipping web-check"; \
	fi

web-build: web-check ## Build the Hugo site into web/public
	@command -v hugo >/dev/null 2>&1 || { \
		echo "hugo not found -- install it: brew install hugo (see https://gohugo.io/installation/)"; \
		exit 1; \
	}
	@hugo_ver=$$(hugo version | sed -n 's/.*hugo v\([0-9]*\.[0-9]*\.[0-9]*\).*/\1/p'); \
	if [ -z "$$hugo_ver" ]; then \
		echo "could not parse hugo version from 'hugo version'"; \
		exit 1; \
	fi; \
	oldest=$$(printf '%s\n%s\n' "$(HUGO_MIN_VERSION)" "$$hugo_ver" | sort -V | head -n1); \
	if [ "$$oldest" != "$(HUGO_MIN_VERSION)" ]; then \
		echo "hugo $$hugo_ver found, but >= $(HUGO_MIN_VERSION) is required -- upgrade: brew upgrade hugo"; \
		exit 1; \
	fi
	@echo "Building Hugo site..."
	@hugo --minify -s web
	@echo "Hugo build complete: web/public"

web: web-build ## Regenerate types, type-check, and build the web UI (web-build -> web-check -> web-types, real prerequisites so `make -j` can't interleave them)
