#!/usr/bin/env bash

set -euo pipefail

usage() {
	cat >&2 <<'EOF'
Usage: scripts/check_agent_network_naming.sh [--base REF] [FILE ...]

Check changed Agent Network files for unapproved legacy product identifiers.
With no FILE arguments, changed tracked and untracked files are discovered from
the working tree. Migration and attribution records are explicitly allowlisted.
EOF
}

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
gate_path="${repo_root}/scripts/check_agent_network_naming.sh"
base_ref=""
files=()
auto_discovery=0

while [ "$#" -gt 0 ]; do
	case "$1" in
		--base)
			if [ "$#" -lt 2 ]; then
				usage
				exit 2
			fi
			base_ref="$2"
			shift 2
			;;
		--help|-h)
			usage
			exit 0
			;;
		--)
			shift
			while [ "$#" -gt 0 ]; do
				files+=("$1")
				shift
			done
			;;
		-*)
			printf 'unknown option: %s\n' "$1" >&2
			usage
			exit 2
			;;
		*)
			files+=("$1")
			shift
			;;
	esac
done

if [ "${#files[@]}" -eq 0 ]; then
	auto_discovery=1
	changed_files=""
	if [ -n "${base_ref}" ]; then
		changed_files="$(git -C "${repo_root}" diff --name-only --diff-filter=ACMRTUXB "${base_ref}" --)"
	else
		changed_files="$(git -C "${repo_root}" diff --name-only --diff-filter=ACMRTUXB HEAD --)"
	fi
	while IFS= read -r file; do
		[ -n "${file}" ] && files+=("${file}")
	done <<< "${changed_files}"

	untracked_files="$(git -C "${repo_root}" ls-files --others --exclude-standard)"
	while IFS= read -r file; do
		[ -n "${file}" ] && files+=("${file}")
	done <<< "${untracked_files}"
fi

is_allowlisted_path() {
	local path="$1"
	case "${path}" in
		CHANGELOG*|*/CHANGELOG*|CHANGELOG.md|*/CHANGELOG.md)
			return 0
			;;
		openspec/*|*/openspec/*)
			return 0
			;;
		*/migration/*|*/migrations/*|*/product-identity-migration/*|migration/*|migrations/*|product-identity-migration/*)
			return 0
			;;
		*-identity-inventory.*|*/product_identity_inventory.md)
			return 0
			;;
		*generated-go*|*types/proto-es*|*proto/store*|*proto/v1/v1*|*proto/gen*)
			return 0
			;;
		LICENSE|LICENSE.*|NOTICE|NOTICE.*|COPYING|COPYING.*|*/LICENSE|*/LICENSE.*|*/NOTICE|*/NOTICE.*|*/COPYING|*/COPYING.*|*/attribution/*|*/ATTRIBUTION/*|*/licenses/*|*/LICENSES/*|*/third_party/*|*/THIRD_PARTY/*)
			return 0
			;;
		*)
			return 1
			;;
	 esac
}

is_test_fixture_path() {
	case "$1" in
		*/tests/fixtures/*|*/testdata/*)
			return 0
			;;
		*)
			return 1
			;;
	esac
}

violations=0
scanned=0
legacy_product='lae''lia'
legacy_module_root="github.com/Ranxy/${legacy_product}"
compatibility_import_re="^[[:space:]]*(import[[:space:]]+|option[[:space:]]+go_package[[:space:]]+=[[:space:]]+)?([[:alnum:]_.]+[[:space:]]+)?\"${legacy_module_root}(/[^\"]*)?\"(;)?([[:space:]]*)$"
filter_compatibility_imports() {
	while IFS= read -r line; do
		if [[ "${line}" =~ ${compatibility_import_re} ]]; then
			continue
		fi
		if [[ "${line}" =~ github\.com/Ranxy/${legacy_product}/backend/generated-go/ ]]; then
			continue
		fi
		printf '%s\n' "${line}"
	done
}
for file in ${files[@]+"${files[@]}"}; do
	case "${file}" in
		/*) absolute_path="${file}" ;;
		*) absolute_path="${repo_root}/${file}" ;;
	esac

	case "${absolute_path}" in
		"${repo_root}"/*) relative_path="${absolute_path#"${repo_root}/"}" ;;
		*)
			printf 'naming gate: file is outside repository: %s\n' "${file}" >&2
			violations=$((violations + 1))
			continue
			;;
	esac

	[ -f "${absolute_path}" ] || continue
	[ "${absolute_path}" = "${gate_path}" ] && continue
	if [ "${auto_discovery}" -eq 1 ] && is_test_fixture_path "${relative_path}"; then
		continue
	fi
	scanned=$((scanned + 1))

	if is_allowlisted_path "${relative_path}"; then
		continue
	fi

	if [ "${auto_discovery}" -eq 1 ] && git -C "${repo_root}" ls-files --error-unmatch "${relative_path}" >/dev/null 2>&1; then
		diff_base="${base_ref:-HEAD}"
		added_lines="$(git -C "${repo_root}" diff --unified=0 "${diff_base}" -- "${relative_path}" | sed -n -e '/^+++ /d' -e 's/^+//p')"
		matches="$(printf '%s\n' "${added_lines}" | filter_compatibility_imports | LC_ALL=C grep -n -E -i "${legacy_product}" || true)"
	else
		matches="$(filter_compatibility_imports <"${absolute_path}" | LC_ALL=C grep -n -E -i "${legacy_product}" || true)"
	fi
	if [ -n "${matches}" ]; then
		printf 'naming gate: unapproved legacy identifier in %s\n%s\n' "${relative_path}" "${matches}" >&2
		violations=$((violations + 1))
	fi
done

if [ "${violations}" -ne 0 ]; then
	printf 'naming gate failed: %d violation(s) across %d file(s)\n' "${violations}" "${scanned}" >&2
	exit 1
fi

printf 'naming gate passed: scanned %d file(s)\n' "${scanned}"
