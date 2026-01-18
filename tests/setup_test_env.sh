#!/bin/bash

# Setup script for running all TableTheory tests including integration and stress tests

set -e

echo "Setting up TableTheory test environment..."

DYNAMODB_LOCAL_IMAGE="${DYNAMODB_LOCAL_IMAGE:-amazon/dynamodb-local:3.1.0}"

# Check if Docker is installed
if ! command -v docker &> /dev/null; then
    echo "Error: Docker is required to run DynamoDB Local"
    echo "Please install Docker from https://www.docker.com/"
    exit 1
fi

# Check if DynamoDB Local container exists
if docker ps -a | grep -q dynamodb-local; then
    # Container exists, check if it's running
    if docker ps | grep -q dynamodb-local; then
        echo "DynamoDB Local is already running"
    else
        echo "Starting existing DynamoDB Local container..."
        docker start dynamodb-local
        echo "Waiting for DynamoDB Local to be ready..."
        sleep 3
    fi
else
    # Container doesn't exist, create it
    echo "Creating and starting DynamoDB Local..."
    docker run -d \
        --name dynamodb-local \
        -p 8000:8000 \
        "${DYNAMODB_LOCAL_IMAGE}" \
        -jar DynamoDBLocal.jar \
        -inMemory \
        -sharedDb
    
    echo "Waiting for DynamoDB Local to be ready..."
    sleep 5
fi

# Test connection to ensure it's accessible
max_attempts=10
attempt=1
while [ $attempt -le $max_attempts ]; do
    if curl -s http://localhost:8000 > /dev/null 2>&1; then
        echo "DynamoDB Local is ready!"
        break
    fi
    echo "Waiting for DynamoDB Local... (attempt $attempt/$max_attempts)"
    sleep 2
    ((attempt++))
done

if [ $attempt -gt $max_attempts ]; then
    echo "Error: DynamoDB Local failed to start or is not accessible"
    echo "Try running: docker logs dynamodb-local"
    exit 1
fi

# Export environment variables
export DYNAMODB_ENDPOINT=http://localhost:8000
export AWS_REGION=us-east-1
export AWS_ACCESS_KEY_ID=dummy
export AWS_SECRET_ACCESS_KEY=dummy

echo ""
echo "Test environment is ready!"
echo ""
echo "To run all tests including integration and stress tests:"
echo "  go test ./... -v"
echo ""
echo "To run only the previously skipped tests:"
echo "  go test ./tests/stress -v"
echo "  go test ./tests/integration -v" 
echo "  go test ./examples/payment/tests -v"
echo ""
echo "To clean up when done:"
echo "  docker stop dynamodb-local && docker rm dynamodb-local" 
