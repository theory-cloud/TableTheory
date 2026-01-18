#!/usr/bin/env bash
set -euo pipefail

# Verifies validator ↔ converter parity at the public boundaries that accept user-provided values.
#
# Goal: avoid "validated but crashes" scenarios (panics) and ensure failures surface as typed errors.
#
# Implementation note:
# - The harness code lives under scripts/internal/ so it can import internal packages.

go run ./scripts/internal/validation_parity

