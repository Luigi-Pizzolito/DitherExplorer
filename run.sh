#!/usr/bin/env bash
set -euo pipefail

if ! command -v air >/dev/null 2>&1; then
  echo "air is not installed. Install it with:"
  echo "  go install github.com/air-verse/air@latest"
  exit 1
fi

exec air -c .air.toml
