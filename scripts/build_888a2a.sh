#!/usr/bin/env bash
# Build 888a2a manager (with embedded frontend; machine binaries are embedded
# by default and can be disabled).
#
# Usage:
#   scripts/build_888a2a.sh [output-dir] [embed-machine]
#   scripts/build_888a2a.sh --no-embed-machine [output-dir]
#   scripts/build_888a2a.sh --release [output-dir]
#
# embed-machine is "true" (default) or "false". You can also pass
# --no-embed-machine / --embed-machine, or set EMBED_MACHINE=true/false.
# When false, the script skips cross-compiling/embedding the per-platform
# machine binaries and builds the manager with only embed_frontend.
#
# release is "false" (default) or "true". You can also pass --release / --dev
# or set RELEASE=true/false. Release builds add the `release` build tag so the
# manager runs in release mode; daily builds default to dev mode.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
. ./scripts/build_init.sh

OUTPUT_DIR="build"
OUTPUT_DIR_SET=0
EMBED_MACHINE="${EMBED_MACHINE:-true}"
RELEASE="${RELEASE:-false}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-embed-machine)
      EMBED_MACHINE=false
      shift
      ;;
    --embed-machine)
      EMBED_MACHINE=true
      shift
      ;;
    --release)
      RELEASE=true
      shift
      ;;
    --dev)
      RELEASE=false
      shift
      ;;
    --help|-h)
      echo "Usage: $0 [--no-embed-machine|--embed-machine] [--release|--dev] [output-dir] [embed-machine]"
      exit 0
      ;;
    *)
      if [[ "${OUTPUT_DIR_SET}" -eq 0 ]]; then
        OUTPUT_DIR="$1"
        OUTPUT_DIR_SET=1
      elif [[ "$1" == "true" || "$1" == "false" ]]; then
        EMBED_MACHINE="$1"
      else
        echo "Unknown argument: $1" >&2
        exit 1
      fi
      shift
      ;;
  esac
done

case "${EMBED_MACHINE}" in
  true|1|yes) EMBED_MACHINE=true ;;
  false|0|no) EMBED_MACHINE=false ;;
  *)
    echo "embed-machine must be true or false (got: ${EMBED_MACHINE})" >&2
    exit 1
    ;;
esac

case "${RELEASE}" in
  true|1|yes) RELEASE=true ;;
  false|0|no) RELEASE=false ;;
  *)
    echo "RELEASE must be true or false (got: ${RELEASE})" >&2
    exit 1
    ;;
esac

EMBED_DIR="backend/manager/server/embedded_machine"
DIST_DIR="backend/manager/server/dist"

# ALWAYS rebuild the frontend on manager builds so the binary embeds the
# latest code (not whatever stale build happened to be in server/dist).
echo "Building frontend..."
pnpm --dir frontend i --frozen-lockfile
pnpm --dir frontend build
rm -rf "${DIST_DIR}"
cp -r frontend/dist "${DIST_DIR}"

BUILD_TAGS="embed_frontend"

if [[ "${EMBED_MACHINE}" == "true" ]]; then
  echo "Building embedded machine binaries..."
  scripts/build-embedded-machines.sh "${EMBED_DIR}"
  BUILD_TAGS="${BUILD_TAGS} embed_machine"
else
  # Ensure no stale machine binaries get embedded when EMBED_MACHINE=false
  rm -rf "${EMBED_DIR}"
fi

# Add release build tag for release-mode builds
if [[ "${RELEASE}" == "true" ]]; then
  BUILD_TAGS="${BUILD_TAGS} release"
fi

mkdir -p "${OUTPUT_DIR}"

BUILD_MODE="dev"
if [[ "${RELEASE}" == "true" ]]; then
  BUILD_MODE="release"
fi
echo "Building manager (${VERSION}, ${BUILD_MODE} mode)..."
CGO_ENABLED=0 go build -tags "${BUILD_TAGS}" -ldflags "-w -s -X github.com/tbdavid2019/888a2a/backend/manager/version.Version=${VERSION} -X github.com/tbdavid2019/888a2a/backend/manager/version.GitCommit=${GIT_COMMIT} -X github.com/tbdavid2019/888a2a/backend/manager/version.BuildTime=${BUILD_TIME}" -p=16 \
	-o "${OUTPUT_DIR}/888a2a" ./backend/manager/bin/server/main.go
legacy_alias="lae""lia"
cp -f "${OUTPUT_DIR}/888a2a" "${OUTPUT_DIR}/${legacy_alias}"

echo ""
echo "Build complete:"
if [[ "${EMBED_MACHINE}" == "true" ]]; then
  echo "  ${OUTPUT_DIR}/888a2a        (manager, ${BUILD_MODE} mode, frontend + machine binaries embedded)"
  echo "  ${OUTPUT_DIR}/${legacy_alias}        (legacy alias)"
  echo "  ${EMBED_DIR}/               (per-platform machine binaries + manifest)"
else
  echo "  ${OUTPUT_DIR}/888a2a        (manager, ${BUILD_MODE} mode, frontend embedded, machine binaries not embedded)"
  echo "  ${OUTPUT_DIR}/${legacy_alias}        (legacy alias)"
fi
