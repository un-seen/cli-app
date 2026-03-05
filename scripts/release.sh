#!/usr/bin/env bash
set -euo pipefail

CONFIG_FILE="configs/hai.yaml"

echo "==> Building hai..."
make build-one CONFIG_FILE="$CONFIG_FILE"

echo "==> Releasing hai to GitHub..."
make release-one CONFIG_FILE="$CONFIG_FILE"
