#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
gate="${repo_root}/scripts/check_agent_network_naming.sh"
fixture_root="${repo_root}/scripts/tests/fixtures/agent-network-naming"

if "${gate}" "${fixture_root}/legacy/runtime.fixture"; then
	printf 'expected an unapproved legacy identifier to be rejected\n' >&2
	exit 1
fi

"${gate}" "${fixture_root}/migration/product-identity-mapping.md"
"${gate}" "${fixture_root}/attribution/NOTICE.txt"

tmp_repo="$(mktemp -d)"
trap 'rm -rf "${tmp_repo}"' EXIT
mkdir -p "${tmp_repo}/scripts"
cp "${gate}" "${tmp_repo}/scripts/check_agent_network_naming.sh"
git -C "${tmp_repo}" init -q
git -C "${tmp_repo}" config user.email naming-gate@example.invalid
git -C "${tmp_repo}" config user.name naming-gate-test
legacy_identifier='lae''lia-agent'
printf 'const legacyName = "%s";\n' "${legacy_identifier}" >"${tmp_repo}/runtime.go"
git -C "${tmp_repo}" add scripts/check_agent_network_naming.sh runtime.go
git -C "${tmp_repo}" commit -qm baseline

printf 'const currentName = "888a2a Agent";\n' >>"${tmp_repo}/runtime.go"
"${tmp_repo}/scripts/check_agent_network_naming.sh"

git -C "${tmp_repo}" checkout -q -- runtime.go
legacy_module='github.com/Ranxy/'"${legacy_identifier%-agent}"
printf 'import _ "%s/backend/common"\n' "${legacy_module}" >>"${tmp_repo}/runtime.go"
"${tmp_repo}/scripts/check_agent_network_naming.sh"
printf 'import _ "%s/backend/common"\n' "${legacy_module}" >"${tmp_repo}/compatibility.go"
"${tmp_repo}/scripts/check_agent_network_naming.sh" "${tmp_repo}/compatibility.go"

git -C "${tmp_repo}" checkout -q -- runtime.go
printf 'const addedLegacyName = "%s";\n' "${legacy_identifier}" >>"${tmp_repo}/runtime.go"
if "${tmp_repo}/scripts/check_agent_network_naming.sh"; then
	printf 'expected a newly added legacy identifier to be rejected\n' >&2
	exit 1
fi

printf 'agent network naming gate fixtures passed\n'
