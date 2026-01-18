#!/usr/bin/env bash
set -euo pipefail

bash scripts/verify-go-file-size.sh
bash scripts/verify-ts-file-size.sh
bash scripts/verify-python-file-size.sh

echo "file-size: PASS"
