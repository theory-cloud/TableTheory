.PHONY: help setup run test clean docker-up docker-down

# Default target
help:
	@echo "Multi-tenant SaaS Example - Available commands:"
	@echo "  make setup       - Install dependencies"
	@echo "  make docker-up   - Start local DynamoDB and Redis"
	@echo "  make docker-down - Stop local services"
	@echo "  make run         - Run the API server locally"
	@echo "  make test        - Run tests"
	@echo "  make clean       - Clean up generated files"

# Install dependencies
setup:
	go mod download
	go install github.com/google/wire/cmd/wire@latest

# Start local services
docker-up:
	docker-compose up -d
	@echo "Waiting for DynamoDB to be ready..."
	@sleep 3
	@echo "DynamoDB is ready at http://localhost:8000"
	@echo "DynamoDB Admin UI at http://localhost:8001"

# Stop local services
docker-down:
	docker-compose down

# Run the API server locally
run:
	AWS_ENDPOINT_URL=http://localhost:8000 \
	AWS_REGION=us-east-1 \
	AWS_ACCESS_KEY_ID=dummy \
	AWS_SECRET_ACCESS_KEY=dummy \
	go run cmd/local/main.go

# Run tests
test:
	AWS_ENDPOINT_URL=http://localhost:8000 \
	AWS_REGION=us-east-1 \
	AWS_ACCESS_KEY_ID=dummy \
	AWS_SECRET_ACCESS_KEY=dummy \
	go test -v ./...

# Run specific test
test-one:
	AWS_ENDPOINT_URL=http://localhost:8000 \
	AWS_REGION=us-east-1 \
	AWS_ACCESS_KEY_ID=dummy \
	AWS_SECRET_ACCESS_KEY=dummy \
	go test -v -run $(TEST) ./...

# Clean up
clean:
	go clean
	rm -rf bin/

# Build for deployment
build:
	GOOS=linux GOARCH=amd64 go build -o bin/bootstrap cmd/lambda/main.go
	cd bin && zip deployment.zip bootstrap

# Deploy to AWS (requires SAM CLI)
deploy: build
	sam deploy --guided

# Format code
fmt:
	go fmt ./...
	gofmt -s -w .

# Lint code
lint:
	golangci-lint run

# Generate mocks (if needed)
generate:
	go generate ./... 