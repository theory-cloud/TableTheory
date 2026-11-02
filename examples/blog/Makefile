.PHONY: help setup test build deploy stop-local clean fmt lint logs shell

# Local DynamoDB port used by `make setup`. Override it when :8000 is taken.
BLOG_DYNAMODB_PORT ?= 8000
DYNAMODB_ENDPOINT ?= http://localhost:$(BLOG_DYNAMODB_PORT)

# This example is its own Go module (it replaces the library with ../..), so
# every target runs from this directory and never initializes a module.

# Default target
help:
	@echo "Available targets:"
	@echo "  setup       - Install dependencies, start local DynamoDB and create tables"
	@echo "  test        - Run the example's unit tests"
	@echo "  build       - Build the Lambda artifacts (posts.zip, comments.zip, authorizer.zip)"
	@echo "  deploy      - Deploy to AWS with SAM"
	@echo "  stop-local  - Stop the local DynamoDB compose stack"
	@echo "  clean       - Remove build artifacts and local data"
	@echo "  fmt         - Format the example module"
	@echo "  lint        - Lint the example module"
	@echo "  logs        - Follow local DynamoDB logs"
	@echo "  shell       - Open the DynamoDB admin UI"
	@echo ""
	@echo "For the full local run (handlers through the Lambda runtime interface),"
	@echo "run 'make examples-local' from the repository root."

# Install dependencies and provision the local tables with the template names.
setup:
	@echo "Installing dependencies..."
	go mod tidy
	@echo "Starting local DynamoDB on port $(BLOG_DYNAMODB_PORT)..."
	cd deployment && BLOG_DYNAMODB_PORT=$(BLOG_DYNAMODB_PORT) docker compose up -d
	@echo "Creating tables..."
	DYNAMODB_ENDPOINT=$(DYNAMODB_ENDPOINT) go run ./cmd/local -mode setup

# Run the module's unit tests. There is no integration build tag in this
# example: the real integration run is the examples harness.
test:
	@echo "Running unit tests..."
	go test ./...

# Build Lambda functions
build:
	@echo "Building Lambda functions..."
	rm -f posts.zip comments.zip authorizer.zip bootstrap
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bootstrap ./cmd/posts
	zip posts.zip bootstrap
	rm bootstrap
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bootstrap ./cmd/comments
	zip comments.zip bootstrap
	rm bootstrap
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bootstrap ./authorizer
	zip authorizer.zip bootstrap
	rm bootstrap

# Deploy to AWS using SAM
deploy: build
	@echo "Deploying to AWS..."
	sam deploy --guided

# Stop local environment
stop-local:
	@echo "Stopping local environment..."
	cd deployment && docker compose down

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -f *.zip bootstrap
	cd deployment && docker compose down -v

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...
	gofmt -s -w .

# Lint code
lint:
	@echo "Linting code..."
	golangci-lint run ./...

# View logs
logs:
	cd deployment && docker compose logs -f

# DynamoDB shell
shell:
	@echo "Opening DynamoDB admin UI..."
	@echo "Visit http://localhost:8001"
	open http://localhost:8001 || xdg-open http://localhost:8001 || echo "Please open http://localhost:8001 in your browser"
