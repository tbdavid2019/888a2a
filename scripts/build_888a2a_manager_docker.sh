#!/usr/bin/env bash
# Build the 888a2a manager docker image (frontend + machine binaries embedded).
#
# Usage:
#   scripts/build_888a2a_manager_docker.sh
#   A2A888_BUILD_PROXY=http://host:port scripts/build_888a2a_manager_docker.sh
#   RELEASE=true scripts/build_888a2a_manager_docker.sh
#   scripts/build_888a2a_manager_docker.sh --release
#
# RELEASE=false (default) builds the manager in dev mode; RELEASE=true (or
# --release) adds the `release` build tag so the manager runs in release mode.
#
# A2A888_BUILD_PROXY routes the Go module download.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
. ./scripts/build_docker_common.sh
collect_common_build_args

RELEASE="${RELEASE:-false}"
if [[ $# -gt 0 ]]; then
  case "$1" in
    --release)
      RELEASE=true
      shift
      ;;
    --dev)
      RELEASE=false
      shift
      ;;
    --help|-h)
      echo "Usage: $0 [--release|--dev]"
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      exit 1
      ;;
  esac
fi
case "${RELEASE}" in
  true|1|yes) RELEASE=true ;;
  false|0|no) RELEASE=false ;;
  *)
    echo "RELEASE must be true or false (got: ${RELEASE})" >&2
    exit 1
    ;;
esac

BUILD_ARGS+=(--build-arg "RELEASE=${RELEASE}")

legacy_repo="lae""lia"
echo "Building 888a2a manager docker image ${VERSION} (${RELEASE} mode)..."
docker build -f ./scripts/docker/Dockerfile.manager \
	"${BUILD_ARGS[@]}" \
	-t "888a2a/manager:${VERSION}" \
	-t "888a2a/manager:latest" \
	-t "${legacy_repo}/manager:${VERSION}" \
	-t "${legacy_repo}/manager:latest" \
	.

echo ""
echo "Image:"
echo "  888a2a/manager:${VERSION}  (run with A2A888_PG_URL pointing at PostgreSQL)"
echo "  ${legacy_repo}/manager:${VERSION}  (legacy alias)"
