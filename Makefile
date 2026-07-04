.PHONY: all build run test clean dev docker-up docker-down migrate-up migrate-down bootstrap help

APP_NAME := fury-sms-gateway
BUILD_DIR := ./build

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

all: clean build ## Clean and build

build: ## Build the API server
	@mkdir -p $(BUILD_DIR)
	go build -ldflags="-X main.version=$(VERSION)" -o $(BUILD_DIR)/$(APP_NAME) ./cmd/api
	@echo "✓ Built $(BUILD_DIR)/$(APP_NAME)"

build-bootstrap: ## Build the bootstrap command
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(APP_NAME)-bootstrap ./cmd/bootstrap
	@echo "✓ Built $(BUILD_DIR)/$(APP_NAME)-bootstrap"

run: build ## Build and run the API server
	./$(BUILD_DIR)/$(APP_NAME)

test: ## Run all tests
	go test -v -race -count=1 ./...

test-short: ## Run short tests only
	go test -v -short -count=1 ./...

test-coverage: ## Run tests with coverage
	go test -v -race -count=1 -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "✓ Coverage report: coverage.html"

clean: ## Clean build artifacts
	rm -rf $(BUILD_DIR) coverage.out coverage.html

lint: ## Run static analysis
	go vet ./...
	@echo "✓ go vet passed"

tidy: ## Tidy Go modules
	go mod tidy
	go mod verify

docker-up: ## Start all services with Docker Compose
	docker compose -f docker/docker-compose.yml up -d

docker-down: ## Stop all services
	docker compose -f docker/docker-compose.yml down

docker-logs: ## View logs
	docker compose -f docker/docker-compose.yml logs -f

bootstrap: build-bootstrap ## Create the first super admin
	./$(BUILD_DIR)/$(APP_NAME)-bootstrap \
		--admin-email="$(EMAIL)" \
		--admin-password="$(PASSWORD)" \
		--admin-name="$(NAME)"

dev: ## Run in development mode with live reload (requires air)
	air

migrate-up: ## Run database migrations (placeholder - migrations run via bootstrap)
	@echo "Use 'make bootstrap' to run migrations and create super admin"

.PHONY: dev
