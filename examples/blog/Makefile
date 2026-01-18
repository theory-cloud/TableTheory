.PHONY: help setup test build deploy clean run-local

# Default target
help:
	@echo "Available targets:"
	@echo "  setup       - Install dependencies and setup local environment"
	@echo "  test        - Run unit and integration tests"
	@echo "  build       - Build Lambda functions"
	@echo "  deploy      - Deploy to AWS"
	@echo "  run-local   - Run locally with docker-compose"
	@echo "  clean       - Clean build artifacts"

# Install dependencies
setup:
	@echo "Installing dependencies..."
	go mod init github.com/theory-cloud/tabletheory/examples/blog || true
	go mod tidy
	@echo "Setting up local DynamoDB..."
	cd deployment && docker-compose up -d
	@echo "Waiting for DynamoDB to start..."
	sleep 5
	@echo "Creating tables..."
	go run cmd/local/setup.go

# Run tests
test:
	@echo "Running unit tests..."
	go test ./tests/... -v -cover
	@echo "Running integration tests..."
	DYNAMODB_ENDPOINT=http://localhost:8000 go test ./tests/... -tags=integration -v

# Run benchmarks
bench:
	@echo "Running benchmarks..."
	go test ./tests/... -bench=. -benchmem

# Build Lambda functions
build:
	@echo "Building Lambda functions..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bootstrap handlers/posts.go
	zip posts.zip bootstrap
	rm bootstrap
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bootstrap handlers/comments.go
	zip comments.zip bootstrap
	rm bootstrap

# Deploy to AWS using SAM
deploy: build
	@echo "Deploying to AWS..."
	sam deploy --guided

# Run locally
run-local:
	@echo "Starting local environment..."
	cd deployment && docker-compose up -d
	@echo "Running local server..."
	DYNAMODB_ENDPOINT=http://localhost:8000 go run cmd/local/main.go

# Stop local environment
stop-local:
	@echo "Stopping local environment..."
	cd deployment && docker-compose down

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -f *.zip bootstrap
	cd deployment && docker-compose down -v

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...
	gofmt -s -w .

# Lint code
lint:
	@echo "Linting code..."
	golangci-lint run ./...

# Create sample data
seed:
	@echo "Creating sample data..."
	DYNAMODB_ENDPOINT=http://localhost:8000 go run cmd/local/seed.go

# View logs
logs:
	cd deployment && docker-compose logs -f

# DynamoDB shell
shell:
	@echo "Opening DynamoDB admin UI..."
	@echo "Visit http://localhost:8001"
	open http://localhost:8001 || xdg-open http://localhost:8001 || echo "Please open http://localhost:8001 in your browser" 
