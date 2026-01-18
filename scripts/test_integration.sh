#!/bin/bash
# Integration test runner for TableTheory
# This script starts DynamoDB Local and runs integration tests

set -e

echo "================================"
echo "TableTheory Integration Test Runner"
echo "================================"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

DYNAMODB_LOCAL_IMAGE="${DYNAMODB_LOCAL_IMAGE:-amazon/dynamodb-local:3.1.0}"

# Check if Docker is available
if ! command -v docker &> /dev/null; then
    echo -e "${RED}Error: Docker is not installed or not in PATH${NC}"
    echo "Please install Docker to run DynamoDB Local"
    exit 1
fi

# Check if DynamoDB Local is already running
if docker ps | grep -q dynamodb-test; then
    echo -e "${YELLOW}DynamoDB Local is already running${NC}"
else
    echo "Starting DynamoDB Local..."
    docker run -d -p 8000:8000 --name dynamodb-test "${DYNAMODB_LOCAL_IMAGE}"
    
    echo "Waiting for DynamoDB Local to start..."
    for i in {1..10}; do
        if curl -s http://localhost:8000 > /dev/null; then
            echo -e "${GREEN}DynamoDB Local is ready${NC}"
            break
        fi
        echo -n "."
        sleep 1
    done
    echo
fi

# Set environment variables
export DYNAMODB_ENDPOINT="http://localhost:8000"
export AWS_REGION="us-east-1"

echo ""
echo "Running integration tests..."
echo "============================"

# Run the new basic integration test first
echo -e "${YELLOW}Running basic integration test...${NC}"
if go test -v ./tests/integration/basic_integration_test.go; then
    echo -e "${GREEN}✓ Basic integration test passed${NC}"
else
    echo -e "${RED}✗ Basic integration test failed${NC}"
    FAILED=true
fi

# If basic test passes, try other tests
if [ -z "$FAILED" ]; then
    echo ""
    echo -e "${YELLOW}Running all integration tests...${NC}"
    go test ./tests/integration/... -v || true
fi

echo ""
echo "Test run complete!"
echo ""

# Ask if user wants to stop DynamoDB Local
read -p "Stop DynamoDB Local? (y/n) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "Stopping DynamoDB Local..."
    docker stop dynamodb-test
    docker rm dynamodb-test
    echo -e "${GREEN}DynamoDB Local stopped${NC}"
else
    echo -e "${YELLOW}DynamoDB Local is still running at http://localhost:8000${NC}"
    echo "To stop it later, run: docker stop dynamodb-test && docker rm dynamodb-test"
fi 
