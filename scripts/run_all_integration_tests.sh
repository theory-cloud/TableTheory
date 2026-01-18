#!/bin/bash
# Run all TableTheory integration tests and summarize results
# This helps identify which tests pass and which need fixes

set -e

echo "========================================"
echo "TableTheory Integration Test Suite Runner"
echo "========================================"
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

DYNAMODB_LOCAL_IMAGE="${DYNAMODB_LOCAL_IMAGE:-amazon/dynamodb-local:3.1.0}"

# Check if Docker is available
if ! command -v docker &> /dev/null; then
    echo -e "${RED}Error: Docker is not installed or not in PATH${NC}"
    exit 1
fi

# Check if DynamoDB Local is running
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
echo -e "${BLUE}Running Integration Tests...${NC}"
echo "============================"

# Track results
PASSED=0
FAILED=0
FAILED_TESTS=()

# Function to run a test and track results
run_test() {
    local test_path=$1
    local test_name=$2
    
    echo -n "Testing $test_name... "
    
    if go test -v "$test_path" > /tmp/test_output_$$.log 2>&1; then
        echo -e "${GREEN}PASSED${NC}"
        ((PASSED++))
    else
        echo -e "${RED}FAILED${NC}"
        ((FAILED++))
        FAILED_TESTS+=("$test_name")
        
        # Show error summary
        echo -e "${YELLOW}  Error summary:${NC}"
        grep -E "(FAIL|panic:|Error:|error:)" /tmp/test_output_$$.log | head -5 | sed 's/^/    /'
    fi
    
    rm -f /tmp/test_output_$$.log
}

# Run each integration test
echo ""
echo -e "${BLUE}Core Integration Tests:${NC}"
echo "----------------------"

run_test "./tests/integration/basic_integration_test.go" "Basic Operations"
run_test "./tests/integration/query_integration_test.go" "Query Integration"
run_test "./tests/integration/update_operations_test.go" "Update Operations"
run_test "./tests/integration/batch_operations_test.go" "Batch Operations"
run_test "./tests/integration/workflow_test.go" "Workflow Tests"
run_test "./tests/integration/migration_test.go" "Migration Tests"
run_test "./tests/integration/migration_largescale_test.go" "Large Scale Migration"

echo ""
echo -e "${BLUE}Stress Tests:${NC}"
echo "------------"

run_test "./tests/stress/concurrent_test.go" "Concurrent Operations"

echo ""
echo -e "${BLUE}Benchmark Tests:${NC}"
echo "---------------"

# Run benchmarks with short duration
if go test -bench=. -benchtime=1s ./tests/benchmarks/query_bench_test.go > /tmp/bench_output_$$.log 2>&1; then
    echo -e "Query Benchmarks... ${GREEN}PASSED${NC}"
    ((PASSED++))
else
    echo -e "Query Benchmarks... ${RED}FAILED${NC}"
    ((FAILED++))
    FAILED_TESTS+=("Query Benchmarks")
fi
rm -f /tmp/bench_output_$$.log

# Summary
echo ""
echo "========================================"
echo -e "${BLUE}Test Summary:${NC}"
echo "========================================"
echo -e "Total Tests Run: $((PASSED + FAILED))"
echo -e "${GREEN}Passed: $PASSED${NC}"
echo -e "${RED}Failed: $FAILED${NC}"

if [ ${#FAILED_TESTS[@]} -gt 0 ]; then
    echo ""
    echo -e "${RED}Failed Tests:${NC}"
    for test in "${FAILED_TESTS[@]}"; do
        echo "  - $test"
    done
fi

echo ""

# Success rate
if [ $((PASSED + FAILED)) -gt 0 ]; then
    SUCCESS_RATE=$((PASSED * 100 / (PASSED + FAILED)))
    echo -e "Success Rate: ${SUCCESS_RATE}%"
    
    if [ $SUCCESS_RATE -eq 100 ]; then
        echo -e "${GREEN}✅ All tests passed!${NC}"
    elif [ $SUCCESS_RATE -ge 80 ]; then
        echo -e "${YELLOW}⚠️  Most tests passed, but some fixes needed${NC}"
    else
        echo -e "${RED}❌ Significant issues found${NC}"
    fi
fi

echo ""

# Ask if user wants to see detailed logs
if [ $FAILED -gt 0 ]; then
    read -p "Run failed tests with verbose output? (y/n) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo ""
        echo -e "${BLUE}Running failed tests with verbose output...${NC}"
        
        # Re-run failed tests with verbose output
        if [[ " ${FAILED_TESTS[@]} " =~ " Basic Operations " ]]; then
            echo -e "\n${YELLOW}Basic Operations Test:${NC}"
            go test -v ./tests/integration/basic_integration_test.go || true
        fi
        
        # Add more as needed...
    fi
fi

echo ""
read -p "Stop DynamoDB Local? (y/n) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    docker stop dynamodb-test
    docker rm dynamodb-test
    echo -e "${GREEN}DynamoDB Local stopped${NC}"
fi 
