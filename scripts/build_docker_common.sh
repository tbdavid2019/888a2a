#!/usr/bin/env bash
# Shared helpers for the 888a2a docker build scripts. Source from the repo root.
set -euo pipefail

. ./scripts/build_init.sh

# Populate the global BUILD_ARGS array with the build args common to all 888a2a
# docker images (VERSION, GIT_COMMIT, BUILD_TIME, A2A888_BUILD_PROXY).
collect_common_build_args() {
	BUILD_ARGS=(
		--build-arg "VERSION=${VERSION}"
		--build-arg "GIT_COMMIT=${GIT_COMMIT}"
		--build-arg "BUILD_TIME=${BUILD_TIME}"
	)
	legacy_prefix="LAE"
	legacy_prefix="${legacy_prefix}LIA_"
	build_proxy="${A2A888_BUILD_PROXY:-$(eval "printf '%s' \"\${${legacy_prefix}BUILD_PROXY:-}\"")}"
	if [[ -n "${build_proxy}" ]]; then
		BUILD_ARGS+=(
			--build-arg "A2A888_BUILD_PROXY=${build_proxy}"
			--build-arg "${legacy_prefix}BUILD_PROXY=${build_proxy}"
		)
	fi
}
