#!/usr/bin/env bash
# Legacy wrapper for scripts/build_888a2a_manager_docker.sh
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
exec ./scripts/build_888a2a_manager_docker.sh "$@"
