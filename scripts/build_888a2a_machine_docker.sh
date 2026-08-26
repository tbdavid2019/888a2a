#!/usr/bin/env bash
# Build the 888a2a machine docker image (pi embedded).
#
# Usage:
#   scripts/build_888a2a_machine_docker.sh
#   A2A888_BUILD_PROXY=http://host:port scripts/build_888a2a_machine_docker.sh
#   APT_MIRROR=http://mirrors.aliyun.com/debian scripts/build_888a2a_machine_docker.sh
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
. ./scripts/build_docker_common.sh
collect_common_build_args

if [[ -n "${APT_MIRROR:-}" ]]; then
	BUILD_ARGS+=(--build-arg "APT_MIRROR=${APT_MIRROR}")
fi
if [[ -n "${CODEX_NPM_SPEC:-}" ]]; then
	BUILD_ARGS+=(--build-arg "CODEX_NPM_SPEC=${CODEX_NPM_SPEC}")
fi

legacy_repo="lae""lia"
echo "Building 888a2a machine docker image ${VERSION}..."
docker build -f ./scripts/docker/Dockerfile.machine \
	"${BUILD_ARGS[@]}" \
	-t "888a2a/machine:${VERSION}" \
	-t "888a2a/machine:latest" \
	-t "${legacy_repo}/machine:${VERSION}" \
	-t "${legacy_repo}/machine:latest" \
	.

echo ""
echo "Image:"
echo "  888a2a/machine:${VERSION}  (run with A2A888_MANAGER_URL; approve the device login printed to the logs)"
echo "  ${legacy_repo}/machine:${VERSION}  (legacy alias)"
