#!/usr/bin/env bash
# Backup, restore, and verify a 888a2a PostgreSQL deployment.
#
# Usage:
#   A2A888_PG_URL=... scripts/backup_888a2a.sh backup <directory>
#   A2A888_PG_URL=... A2A888_RESTORE_CONFIRM=YES \
#     scripts/backup_888a2a.sh restore <dump-file>
#   scripts/backup_888a2a.sh verify <backup-directory>
set -euo pipefail

usage() {
	cat >&2 <<'EOF'
Usage:
  A2A888_PG_URL=... scripts/backup_888a2a.sh backup <directory>
  A2A888_PG_URL=... A2A888_RESTORE_CONFIRM=YES scripts/backup_888a2a.sh restore <dump-file>
  scripts/backup_888a2a.sh verify <backup-directory>

The restore command is destructive and requires A2A888_RESTORE_CONFIRM=YES.
Use a fresh isolated database for a disaster-recovery drill.
EOF
}

checksum() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1"
	else
		shasum -a 256 "$1"
	fi
}

require_command() {
	command -v "$1" >/dev/null 2>&1 || {
		echo "required command not found: $1" >&2
		exit 1
	}
}

backup() {
	local output_dir="$1"
	local timestamp dump schema counts
	timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
	dump="${output_dir}/888a2a-${timestamp}.dump"
	schema="${output_dir}/888a2a-${timestamp}.schema.sql"
	counts="${output_dir}/888a2a-${timestamp}.counts.tsv"

	require_command pg_dump
	require_command psql
	: "${A2A888_PG_URL:?A2A888_PG_URL must be set}"
	mkdir -p "${output_dir}"
	pg_dump --format=custom --no-owner --no-privileges --file "${dump}" "${A2A888_PG_URL}"
	pg_dump --schema-only --no-owner --no-privileges --file "${schema}" "${A2A888_PG_URL}"
	psql --no-psqlrc --tuples-only --no-align --field-separator $'\t' "${A2A888_PG_URL}" \
		-c "SELECT table_name, (xpath('/row/c/text()', query_to_xml(format('SELECT count(*) AS c FROM %I', table_name), true, false, '')))[1]::text::bigint FROM information_schema.tables WHERE table_schema = 'public' ORDER BY table_name;" \
		> "${counts}"
	checksum "${dump}" > "${dump}.sha256"
	checksum "${schema}" > "${schema}.sha256"
	printf 'backup=%s\ncounts=%s\n' "${dump}" "${counts}"
}

restore() {
	local dump="$1"
	require_command pg_restore
	: "${A2A888_PG_URL:?A2A888_PG_URL must be set}"
	if [[ "${A2A888_RESTORE_CONFIRM:-}" != "YES" ]]; then
		echo "refusing destructive restore; set A2A888_RESTORE_CONFIRM=YES" >&2
		exit 2
	fi
	[[ -f "${dump}" ]] || { echo "dump not found: ${dump}" >&2; exit 1; }
	if [[ -f "${dump}.sha256" ]]; then
		( cd "$(dirname "${dump}")" && checksum "$(basename "${dump}")" | cmp - "$(basename "${dump}").sha256" )
	fi
	pg_restore --exit-on-error --clean --if-exists --no-owner --no-privileges \
		--dbname "${A2A888_PG_URL}" "${dump}"
	echo "restore complete: ${dump}"
}

verify() {
	local backup_dir="$1"
	require_command pg_restore
	local dump
	dump="$(find "${backup_dir}" -maxdepth 1 -type f -name '*.dump' -print | sort | tail -n 1)"
	[[ -n "${dump}" ]] || { echo "no custom-format dump found in ${backup_dir}" >&2; exit 1; }
	if [[ -f "${dump}.sha256" ]]; then
		( cd "$(dirname "${dump}")" && checksum "$(basename "${dump}")" | cmp - "$(basename "${dump}").sha256" )
	fi
	pg_restore --list "${dump}" | grep -q 'TABLE.*organizations'
	pg_restore --list "${dump}" | grep -q 'TABLE.*a2a888_approval_request'
	pg_restore --list "${dump}" | grep -q 'TABLE.*a2a888_connector_credential'
	pg_restore --list "${dump}" | grep -q 'TABLE.*a2a888_usage_event'
	echo "backup manifest verified: ${dump}"
}

[[ "$#" -ge 2 ]] || { usage; exit 2; }
case "$1" in
	backup) backup "$2" ;;
	restore) restore "$2" ;;
	verify) verify "$2" ;;
	*) usage; exit 2 ;;
esac
