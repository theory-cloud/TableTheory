#!/usr/bin/env bash
set -euo pipefail

# Verifies public API contract parity for exported helpers (high-risk domain).
#
# Primary target:
# - `tabletheory.UnmarshalItem` and friends must respect canonical TableTheory tag semantics
#   (pk/sk + naming/attr overrides) and fail closed for encrypted envelopes when no
#   decryption context is available.

go run ./scripts/internal/public_api_contracts

