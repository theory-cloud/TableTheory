#!/bin/bash

# TableTheory Test Coverage Dashboard
# Shows current test coverage status and progress

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to get coverage for a package
get_coverage() {
    local pkg=$1
    local coverage=$(go test -cover "./$pkg" 2>&1 | grep -oE '[0-9]+\.[0-9]%' | head -1 || echo "0.0%")
    echo "${coverage:-0.0%}"
}

# Function to get line count for untested packages
get_lines() {
    local pkg=$1
    find "./$pkg" -name "*.go" -not -name "*_test.go" 2>/dev/null | xargs wc -l 2>/dev/null | tail -1 | awk '{print $1}' || echo "0"
}

# Function to display coverage with color
display_coverage() {
    local pkg=$1
    local current=$2
    local target=$3
    local coverage_num=$(echo $current | sed 's/%//')
    
    # Convert to integer for comparison
    coverage_int=${coverage_num%.*}
    
    if [ "$coverage_int" -eq 0 ]; then
        echo -e "${RED}$current${NC}"
    elif [ "$coverage_int" -ge "$target" ]; then
        echo -e "${GREEN}$current${NC}"
    elif [ "$coverage_int" -ge 50 ]; then
        echo -e "${YELLOW}$current${NC}"
    else
        echo -e "${RED}$current${NC}"
    fi
}

# Header
echo -e "${BLUE}======================================"
echo -e "   TableTheory Test Coverage Dashboard   "
echo -e "======================================${NC}"
echo ""
date
echo ""

# Run tests with coverage
echo "Running tests..."
go test -coverprofile=coverage.out ./pkg/... ./internal/... 2>/dev/null || true

# Overall coverage
if [ -f coverage.out ]; then
    overall=$(go tool cover -func=coverage.out 2>/dev/null | grep "total:" | awk '{print $3}' || echo "0.0%")
else
    overall="0.0%"
fi

echo -e "${BLUE}Overall Coverage:${NC} $(display_coverage "overall" "$overall" 75)"
echo ""

# Package status table
echo -e "${BLUE}Package Coverage Status:${NC}"
echo "┌─────────────────────┬──────────┬──────────┬────────┬──────────┐"
echo "│ Package             │ Current  │ Target   │ Lines  │ Status   │"
echo "├─────────────────────┼──────────┼──────────┼────────┼──────────┤"

# Define packages and their targets
declare -A targets=(
    ["pkg/types"]=80
    ["pkg/marshal"]=80
    ["pkg/core"]=85
    ["pkg/errors"]=90
    ["pkg/index"]=80
    ["pkg/session"]=80
    ["pkg/model"]=76
    ["pkg/transaction"]=74
    ["pkg/query"]=70
    ["internal/expr"]=70
)

# Track totals
total_lines=0
tested_lines=0

# Check each package
for pkg in pkg/types pkg/marshal pkg/core pkg/errors pkg/index pkg/session pkg/model pkg/transaction pkg/query internal/expr; do
    coverage=$(get_coverage "$pkg")
    target=${targets[$pkg]}
    lines=$(get_lines "$pkg")
    total_lines=$((total_lines + lines))
    
    coverage_num=$(echo $coverage | sed 's/%//')
    
    # Calculate tested lines (approximate)
    coverage_int=${coverage_num%.*}
    if [ "$coverage_int" -gt 0 ]; then
        tested=$((lines * coverage_int / 100))
        tested_lines=$((tested_lines + tested))
        status="✓"
        if [ "$coverage_int" -lt "$target" ]; then
            status="⚠"
        fi
    else
        status="✗"
    fi
    
    # Format package name
    pkg_display=$(printf "%-19s" "$pkg")
    
    # Display row
    echo -n "│ $pkg_display │ "
    printf "%8s" "$(display_coverage "$pkg" "$coverage" "$target")"
    echo -n " │ "
    printf "%8s" "$target%"
    echo -n " │ "
    printf "%6s" "$lines"
    echo -n " │ "
    
    if [ "$status" = "✓" ]; then
        echo -e "   ${GREEN}$status${NC}      │"
    elif [ "$status" = "⚠" ]; then
        echo -e "   ${YELLOW}$status${NC}      │"
    else
        echo -e "   ${RED}$status${NC}      │"
    fi
done

echo "└─────────────────────┴──────────┴──────────┴────────┴──────────┘"

# Summary statistics
echo ""
echo -e "${BLUE}Summary:${NC}"
echo "• Total lines of code: $total_lines"
echo "• Estimated tested lines: $tested_lines"
echo "• Untested lines: $((total_lines - tested_lines))"

# Progress tracking
echo ""
echo -e "${BLUE}Progress Tracking:${NC}"

# Count packages by status
packages_done=0
packages_partial=0
packages_todo=0

for pkg in "${!targets[@]}"; do
    coverage=$(get_coverage "$pkg")
    coverage_num=$(echo $coverage | sed 's/%//')
    target=${targets[$pkg]}
    
    coverage_int=${coverage_num%.*}
    if [ "$coverage_int" -ge "$target" ]; then
        packages_done=$((packages_done + 1))
    elif [ "$coverage_int" -gt 0 ]; then
        packages_partial=$((packages_partial + 1))
    else
        packages_todo=$((packages_todo + 1))
    fi
done

echo "• Packages at target: $packages_done/10"
echo "• Packages in progress: $packages_partial/10"
echo "• Packages not started: $packages_todo/10"

# Next steps
echo ""
echo -e "${BLUE}Priority Actions:${NC}"

# Find packages needing work
for pkg in pkg/types pkg/marshal pkg/errors pkg/core; do
    coverage=$(get_coverage "$pkg")
    coverage_num=$(echo $coverage | sed 's/%//')
    target=${targets[$pkg]}
    
    coverage_int=${coverage_num%.*}
    if [ "$coverage_int" -lt "$target" ]; then
        needed=$((target - coverage_int))
        echo "• $pkg: needs ${needed}% more coverage"
    fi
done

# Show failing tests
echo ""
echo -e "${BLUE}Test Health:${NC}"
failing_tests=$(go test ./... 2>&1 | grep -E "FAIL:|--- FAIL:" | wc -l || echo "0")
if [ "$failing_tests" -gt 0 ]; then
    echo -e "${RED}• $failing_tests failing tests detected${NC}"
else
    echo -e "${GREEN}• All tests passing${NC}"
fi

echo ""
echo "Run 'make coverage' to see detailed HTML report"
echo "" 