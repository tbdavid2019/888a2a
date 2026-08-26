#!/usr/bin/env bash
# 888a2a-machine docker entrypoint: maps environment variables to CLI flags.
# The machine authenticates via the OAuth2 device-code flow: `setup` prints an
# approval URL to the container logs and waits for a logged-in user to approve
# it in a browser, then continues as the foreground supervisor process (it is
# PID 1 in the container, so it must not daemonize). No token is ever baked
# into an image or command line.
# A2A888_HOME selects the machine data root.
set -euo pipefail

LEGACY_PREFIX="LAE"
LEGACY_PREFIX="${LEGACY_PREFIX}LIA_"

MANAGER_URL="${A2A888_MANAGER_URL:-$(eval "printf '%s' \"\${${LEGACY_PREFIX}MANAGER_URL:-}\"")}"
INSECURE="${A2A888_INSECURE:-$(eval "printf '%s' \"\${${LEGACY_PREFIX}INSECURE:-false}\"")}"
DEBUG="${A2A888_DEBUG:-$(eval "printf '%s' \"\${${LEGACY_PREFIX}DEBUG:-false}\"")}"
CODEX_HOME_OVERRIDE="${A2A888_CODEX_HOME:-$(eval "printf '%s' \"\${${LEGACY_PREFIX}CODEX_HOME:-}\"")}"

args=(setup --no-browser --foreground)
if [[ -n "${MANAGER_URL}" ]]; then
	args+=(--manager "${MANAGER_URL}")
	if [[ "${MANAGER_URL}" == http://* ]]; then
		args+=(--allow-http)
	fi
fi
if [[ "${INSECURE}" == "true" ]]; then
	args+=(--insecure)
fi
if [[ "${DEBUG}" == "true" ]]; then
	args+=(--debug)
fi
# Codex provider login/config: point CODEX_HOME at a mounted writable volume
# carrying config.toml + auth/models.json.
if [[ -n "${CODEX_HOME_OVERRIDE}" ]]; then
	export CODEX_HOME="${CODEX_HOME_OVERRIDE}"
fi

BIN="888a2a-machine"
if ! command -v "${BIN}" >/dev/null 2>&1; then
	BIN="lae""lia-machine"
fi

exec "${BIN}" "${args[@]}" "$@"
