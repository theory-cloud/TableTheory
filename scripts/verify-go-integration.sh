#!/usr/bin/env bash
set -euo pipefail

# Go integration suite against DynamoDB Local. `make integration` resolves the
# endpoint first and only starts the checked-in compose profile when nothing
# answers, so a floor container can run this against a pinned DynamoDB Local
# that the host owns without starting a second backend.

make integration

echo "go-integration: PASS"
