# TableTheory Makefile

SHELL := /bin/bash

.PHONY: all build test test-unit unit-cover clean lint fmt fmt-check docker-up docker-down docker-clean integration contract-tests example-local generate-contract-models verify-generated-models generate-api-reference verify-api-reference sync-runtime-docs-site verify-runtime-docs-site benchmark stress test-all verify-coverage verify-go-modules verify-ci-toolchain verify-planning-docs sec rubric rubric-fast stage-theorycloud-tabletheory-subtree verify-theorycloud-tabletheory-subtree sync-theorycloud-tabletheory-subtree trigger-theorycloud-publish

# Variables
GOMOD := github.com/theory-cloud/tabletheory
TOOLCHAIN := $(shell awk '/^toolchain / {print $$2}' go.mod | head -n 1)
export GOTOOLCHAIN ?= $(TOOLCHAIN)
GO_ENV_GOBIN := $(shell go env GOBIN 2>/dev/null)
GO_ENV_GOPATH := $(shell go env GOPATH 2>/dev/null)
GO_BIN_DIR := $(if $(GO_ENV_GOBIN),$(GO_ENV_GOBIN),$(GO_ENV_GOPATH)/bin)
GOLANGCI_LINT := $(GO_BIN_DIR)/golangci-lint
UNIT_PACKAGES := $(shell go list ./... | grep -v /vendor/ | grep -v /node_modules/ | grep -v /examples/ | grep -v /tests/stress | grep -v /tests/integration)
ALL_PACKAGES := $(shell go list ./... | grep -v /vendor/ | grep -v /node_modules/ | grep -v /examples/ | grep -v /tests/stress)
INTEGRATION_PACKAGES := $(shell go list ./tests/integration/...)
DYNAMODB_LOCAL_IMAGE ?= amazon/dynamodb-local:3.1.0

# Default target
all: fmt lint test build

# Build the project
build:
	@echo "Building TableTheory..."
	@go build -v ./...

# Run all tests (unit + integration)
test: docker-up
	@echo "Running all tests (unit + integration)..."
	@go test -v -race -coverprofile=coverage.out -count=1 $(ALL_PACKAGES)
	@echo ""
	@echo "✅ Success All tests passed!"

# Run only unit tests (fast, no DynamoDB required)
test-unit:
	@echo "Running unit tests only..."
	@go test -v -race -coverprofile=coverage.out $(UNIT_PACKAGES)

unit-cover:
	@echo "Running offline unit coverage..."
	@go test ./... -short -coverpkg=./... -coverprofile=coverage_unit.out

verify-coverage:
	@./scripts/verify-coverage.sh

# Run integration tests (requires DynamoDB Local)
integration: docker-up
	@echo "Running integration tests..."
	@go test -v $(INTEGRATION_PACKAGES)

# Run the shared Go/TypeScript/Python contract suite (requires DynamoDB Local)
contract-tests: docker-up
	@bash scripts/verify-contract-tests.sh

# Run the checked-in Go local quickstart against DynamoDB Local.
example-local: docker-up
	@AWS_ACCESS_KEY_ID=dummy AWS_SECRET_ACCESS_KEY=dummy AWS_REGION=us-east-1 \
		DYNAMODB_ENDPOINT=$${DYNAMODB_ENDPOINT:-http://localhost:8000} \
		go run ./examples/local-quickstart

# Generate and verify DMS-derived contract-runner model files.
generate-contract-models:
	@bash scripts/verify-generated-models.sh --write

verify-generated-models:
	@bash scripts/verify-generated-models.sh --check

# Generate and verify API references from Go/TypeScript/Python source.
generate-api-reference:
	@python3 scripts/generate-api-reference.py --write
	@python3 scripts/sync-runtime-docs-site.py --write

verify-api-reference:
	@python3 scripts/generate-api-reference.py --check
	@python3 scripts/sync-runtime-docs-site.py --check

# Generate and verify Jekyll site copies of package-local TS/Py runtime docs.
sync-runtime-docs-site:
	@python3 scripts/sync-runtime-docs-site.py --write

verify-runtime-docs-site:
	@python3 scripts/sync-runtime-docs-site.py --check

# Run benchmarks
benchmark:
	@echo "Running benchmarks..."
	@go test -bench=. -benchmem ./tests/benchmarks/...

# Run stress tests
stress:
	@echo "Running stress tests..."
	@go test -v ./tests/stress/...

# Run all tests including integration, benchmarks, and stress
test-all: docker-up
	@echo "Running all tests (unit, integration, benchmarks, stress)..."
	@go test -v -race -coverprofile=coverage.out $(ALL_PACKAGES)
	@go test -bench=. -benchmem ./tests/benchmarks/...
	@go test -v ./tests/stress/...
	@echo ""
	@echo "✅ All tests completed!"
	@echo "Note: Run 'make docker-down' to stop DynamoDB Local when done"

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@files="$$(git ls-files '*.go' | while read -r f; do if [ -f "$$f" ]; then echo "$$f"; fi; done)"; \
	if [ -n "$$files" ]; then \
		gofmt -s -w $$files; \
	fi

fmt-check:
	@./scripts/fmt-check.sh

# Run linters
lint:
	@echo "Running linters..."
	@"$(GOLANGCI_LINT)" run --timeout=5m --config .golangci-v2.yml ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -f coverage*.out coverage.html gosec.sarif
	@rm -f py/coverage.xml py/coverage-*.txt py/coverage-*.json py/.coverage
	@rm -f gov-infra/evidence/*-output.log gov-infra/evidence/DOC-5-parity.log
	@rm -rf ts/coverage ts/dist py/build py/dist build/lambda
	@go clean -cache

# Start DynamoDB Local
docker-up:
	@echo "Starting DynamoDB Local..."
	@if curl -s http://localhost:8000 > /dev/null 2>&1; then \
		echo "DynamoDB Local is already running"; \
	else \
		if [ -f docker-compose.yml ]; then \
			if command -v docker-compose > /dev/null 2>&1; then \
				docker-compose up -d dynamodb-local; \
			else \
				docker compose up -d dynamodb-local; \
			fi; \
		else \
			if docker ps -a --format '{{.Names}}' | grep -qx 'dynamodb-local'; then \
				docker start dynamodb-local; \
			else \
				docker run -d --name dynamodb-local -p 8000:8000 $(DYNAMODB_LOCAL_IMAGE) -jar DynamoDBLocal.jar -inMemory -sharedDb; \
			fi; \
		fi; \
	fi
	@echo "Waiting for DynamoDB Local to be ready..."
	@for i in 1 2 3 4 5 6 7 8 9 10; do \
		if curl -s http://localhost:8000 > /dev/null 2>&1; then \
			echo "✅ DynamoDB Local is ready!"; \
			break; \
		fi; \
		if [ $$i -eq 10 ]; then \
			echo "❌ Error: DynamoDB Local failed to start"; \
			exit 1; \
		fi; \
		echo "  Waiting... (attempt $$i/10)"; \
		sleep 2; \
	done

# Stop DynamoDB Local
docker-down:
	@echo "Stopping DynamoDB Local..."
	@if [ -f docker-compose.yml ]; then \
		if command -v docker-compose > /dev/null 2>&1; then \
			docker-compose stop dynamodb-local; \
		else \
			docker compose stop dynamodb-local; \
		fi; \
	elif docker ps --format '{{.Names}}' | grep -qx 'dynamodb-local'; then \
		docker stop dynamodb-local; \
	else \
		echo "DynamoDB Local is not running"; \
	fi

# Remove DynamoDB Local container (useful for cleanup)
docker-clean:
	@echo "Removing DynamoDB Local container..."
	@if [ -f docker-compose.yml ]; then \
		if command -v docker-compose > /dev/null 2>&1; then \
			docker-compose down; \
		else \
			docker compose down; \
		fi; \
		echo "Containers removed"; \
	elif docker ps -a --format '{{.Names}}' | grep -qx 'dynamodb-local'; then \
		docker rm -f dynamodb-local; \
		echo "Container removed"; \
	else \
		echo "No dynamodb-local container found"; \
	fi

# Install development dependencies
install-tools:
	@echo "Installing development tools..."
	@GOBIN="$(GO_BIN_DIR)" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0
	@GOBIN="$(GO_BIN_DIR)" go install github.com/golang/mock/mockgen@latest

# Generate mocks
generate:
	@echo "Generating mocks..."
	@go generate ./...

# Check for compilation errors
check:
	@echo "Checking for compilation errors..."
	@go build -o /dev/null ./... 2>&1 | grep -E "^#|error" || echo "✅ No compilation errors"

verify-go-modules:
	@./scripts/verify-go-modules.sh

verify-ci-toolchain:
	@./scripts/verify-ci-toolchain.sh

verify-planning-docs:
	@./scripts/verify-planning-docs.sh

# Smoke test the `tabletheory init` Go scaffold against DynamoDB Local (requires Docker).
init-smoke:
	@bash ./scripts/verify-init-scaffold.sh

# Dry-run proof that the tabletheory CLI release-asset matrix builds (no publish).
cli-release-dryrun:
	@bash ./scripts/verify-cli-release-assets.sh

sec:
	@./scripts/sec-gosec.sh
	@./scripts/sec-govulncheck.sh
	@go mod verify

rubric:
	@./scripts/verify-rubric.sh

rubric-fast:
	@SKIP_INTEGRATION=true bash ./scripts/verify-rubric-fast.sh

stage-theorycloud-tabletheory-subtree:
	@bash ./scripts/stage_theorycloud_tabletheory_subtree.sh --output "$${THEORYCLOUD_TABLETHEORY_SUBTREE_OUTPUT_DIR:-/tmp/theorycloud-tabletheory-source}"

verify-theorycloud-tabletheory-subtree:
	@bash ./scripts/verify-theorycloud-tabletheory-subtree.sh "$${THEORYCLOUD_TABLETHEORY_SUBTREE_OUTPUT_DIR:-/tmp/theorycloud-tabletheory-source}"

sync-theorycloud-tabletheory-subtree:
	@bash ./scripts/sync_theorycloud_tabletheory_subtree.sh --output "$${THEORYCLOUD_TABLETHEORY_SUBTREE_OUTPUT_DIR:-/tmp/theorycloud-tabletheory-source}"

trigger-theorycloud-publish:
	@bash ./scripts/trigger_theorycloud_publish.sh

# Show test coverage in browser
coverage: test
	@echo "Generating coverage report..."
	@go tool cover -html=coverage.out

# Show coverage dashboard
coverage-dashboard:
	@./scripts/coverage-dashboard.sh

# Quick test for development (unit tests only, no race detector)
quick-test:
	@echo "Running quick tests (unit only, no race detector)..."
	@go test -short $(UNIT_PACKAGES)

# Team 1 specific targets
team1-test:
	@echo "Running Team 1 tests..."
	@go test -v ./pkg/core/... ./pkg/model/... ./pkg/types/... ./pkg/session/... ./pkg/errors/...

# Team 2 specific targets
team2-test:
	@echo "Running Team 2 tests..."
	@go test -v ./pkg/query/... ./internal/expr/... ./pkg/index/...

# Test examples separately
examples-test:
	@echo "Running example tests..."
	@go test -v ./examples/...

# Help target
help:
	@echo "TableTheory Makefile Commands:"
	@echo ""
	@echo "Build & Test:"
	@echo "  make build       - Build the project"
	@echo "  make test        - Run ALL tests (unit + integration) [STARTS DynamoDB Local]"
	@echo "  make test-unit   - Run only unit tests (fast, no Docker required)"
	@echo "  make integration - Run integration tests only (requires Docker)"
	@echo "  make contract-tests - Run Go/TypeScript/Python contract suite (requires Docker)"
	@echo "  make example-local - Start DynamoDB Local and run the Go quickstart CRUD proof"
	@echo "  make verify-runtime-docs-site - Check generated TS/Py site doc copies"
	@echo "  make verify-api-reference - Check generated API reference drift"
	@echo "  make test-all    - Run all tests including benchmarks and stress tests"
	@echo "  make benchmark   - Run performance benchmarks"
	@echo "  make stress      - Run stress tests"
	@echo ""
	@echo "Code Quality:"
	@echo "  make fmt         - Format code"
	@echo "  make fmt-check   - Verify formatting is clean"
	@echo "  make lint        - Run linters"
	@echo "  make check       - Check for compilation errors"
	@echo "  make coverage    - Show test coverage in browser"
	@echo "  make coverage-dashboard - Show coverage dashboard in terminal"
	@echo "  make verify-coverage - Verify library coverage threshold"
	@echo "  make verify-go-modules - Compile all Go modules"
	@echo "  make verify-ci-toolchain - Verify CI toolchain alignment"
	@echo "  make verify-planning-docs - Verify planning docs exist"
	@echo "  make init-smoke  - Smoke test the 'tabletheory init' Go scaffold (requires Docker)"
	@echo "  make cli-release-dryrun - Cross-compile+verify the tabletheory CLI release assets (no publish)"
	@echo "  make sec         - Run security gates (gosec + govulncheck + go mod verify)"
	@echo "  make rubric      - Run full rubric gate set"
	@echo "  make rubric-fast - Run fast contributor gates (format/lint/unit/docs; no Docker)"
	@echo "  make stage-theorycloud-tabletheory-subtree - Stage the theorycloud/tabletheory subtree locally"
	@echo "  make verify-theorycloud-tabletheory-subtree - Verify the staged theorycloud/tabletheory subtree"
	@echo "  make sync-theorycloud-tabletheory-subtree - Sync the staged theorycloud/tabletheory subtree to the configured stage prefix"
	@echo "  make trigger-theorycloud-publish - Trigger the shared theorycloud publish endpoint for the configured stage"
	@echo ""
	@echo "Docker/DynamoDB:"
	@echo "  make docker-up   - Start DynamoDB Local"
	@echo "  make docker-down - Stop DynamoDB Local"
	@echo "  make docker-clean - Remove DynamoDB Local container"
	@echo ""
	@echo "Team/Specific Tests:"
	@echo "  make team1-test  - Run Team 1 specific tests"
	@echo "  make team2-test  - Run Team 2 specific tests"
	@echo "  make examples-test - Run example tests"
	@echo ""
	@echo "Lambda:"
	@echo "  make lambda-build - Build Lambda function example"
	@echo "  make lambda-test  - Test Lambda functionality"
	@echo "  make lambda-bench - Run Lambda benchmarks"
	@echo ""
	@echo "  make help        - Show this help message"

# Lambda-specific targets
LAMBDA_BUILD_FLAGS = -tags lambda -ldflags="-s -w"
GOOS = linux
GOARCH = amd64

# Build Lambda function example
lambda-build:
	@echo "Building Lambda function..."
	@mkdir -p build/lambda
	@cd examples/lambda && GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LAMBDA_BUILD_FLAGS) \
		-o ../../build/lambda/bootstrap main.go
	@cd build/lambda && zip function.zip bootstrap
	@echo "Lambda function built: build/lambda/function.zip"

# Test Lambda functionality
lambda-test:
	@echo "Running Lambda tests..."
	@go test -v ./lambda_test.go -run TestLambda

# Run Lambda benchmarks
lambda-bench:
	@echo "Running Lambda benchmarks..."
	@go test -bench=BenchmarkLambda -benchmem ./lambda_test.go 
