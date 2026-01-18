#!/usr/bin/env bash
set -euo pipefail

# Verifies expression boundary hardening rules for DynamoDB document paths:
# - List indices must never be interpolated from raw strings (injection-by-construction).
# - Invalid / malicious list index paths are rejected at the Builder boundary.

go run ./scripts/internal/expression_hardening
