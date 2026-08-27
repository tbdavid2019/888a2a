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

checksum_value() {
	checksum "$1" | awk '{print $1}'
}

require_command() {
	command -v "$1" >/dev/null 2>&1 || {
		echo "required command not found: $1" >&2
		exit 1
	}
}

database_command_available() {
	command -v "$1" >/dev/null 2>&1 && return 0
	command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1
}

database_exec() {
	if command -v "$1" >/dev/null 2>&1; then
		"$@"
		return
	fi
	docker compose exec -T "${A2A888_DB_SERVICE:-db}" "$@"
}

backup() {
	local output_dir="$1"
	local timestamp dump schema counts
	timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
	dump="${output_dir}/888a2a-${timestamp}.dump"
	schema="${output_dir}/888a2a-${timestamp}.schema.sql"
	counts="${output_dir}/888a2a-${timestamp}.counts.tsv"

	database_command_available pg_dump || { echo "pg_dump or Docker Compose is required" >&2; exit 1; }
	database_command_available psql || { echo "psql or Docker Compose is required" >&2; exit 1; }
	: "${A2A888_PG_URL:?A2A888_PG_URL must be set}"
	mkdir -p "${output_dir}"
	database_exec pg_dump --format=custom --no-owner --no-privileges --file "-" "${A2A888_PG_URL}" > "${dump}"
	database_exec pg_dump --schema-only --no-owner --no-privileges --file "-" "${A2A888_PG_URL}" > "${schema}"
	database_exec psql --no-psqlrc --tuples-only --no-align --field-separator $'\t' "${A2A888_PG_URL}" \
		-c "SELECT table_name, (xpath('/row/c/text()', query_to_xml(format('SELECT count(*) AS c FROM %I', table_name), true, false, '')))[1]::text::bigint FROM information_schema.tables WHERE table_schema = 'public' ORDER BY table_name;" \
		> "${counts}"
	checksum_value "${dump}" > "${dump}.sha256"
	checksum_value "${schema}" > "${schema}.sha256"
	printf 'backup=%s\ncounts=%s\n' "${dump}" "${counts}"
}

restore() {
	local dump="$1"
	database_command_available pg_restore || { echo "pg_restore or Docker Compose is required" >&2; exit 1; }
	: "${A2A888_PG_URL:?A2A888_PG_URL must be set}"
	if [[ "${A2A888_RESTORE_CONFIRM:-}" != "YES" ]]; then
		echo "refusing destructive restore; set A2A888_RESTORE_CONFIRM=YES" >&2
		exit 2
	fi
	[[ -f "${dump}" ]] || { echo "dump not found: ${dump}" >&2; exit 1; }
	if [[ -f "${dump}.sha256" ]]; then
		actual="$(checksum_value "${dump}")"
		expected="$(tr -d '[:space:]' < "${dump}.sha256")"
		[[ "${actual}" == "${expected}" ]] || { echo "dump checksum mismatch" >&2; exit 1; }
	fi
	if command -v pg_restore >/dev/null 2>&1; then
		pg_restore --exit-on-error --clean --if-exists --no-owner --no-privileges \
			--dbname "${A2A888_PG_URL}" "${dump}"
	else
		cat "${dump}" | docker compose exec -T "${A2A888_DB_SERVICE:-db}" \
			pg_restore --exit-on-error --clean --if-exists --no-owner --no-privileges \
			--dbname "${A2A888_PG_URL}" -
	fi
	echo "restore complete: ${dump}"
}

verify() {
	local backup_dir="$1"
	database_command_available pg_restore || { echo "pg_restore or Docker Compose is required" >&2; exit 1; }
	local dump
	dump="$(find "${backup_dir}" -maxdepth 1 -type f -name '*.dump' -print | sort | tail -n 1)"
	[[ -n "${dump}" ]] || { echo "no custom-format dump found in ${backup_dir}" >&2; exit 1; }
	if [[ -f "${dump}.sha256" ]]; then
		actual="$(checksum_value "${dump}")"
		expected="$(tr -d '[:space:]' < "${dump}.sha256")"
		[[ "${actual}" == "${expected}" ]] || { echo "dump checksum mismatch" >&2; exit 1; }
	fi
	if command -v pg_restore >/dev/null 2>&1; then
		pg_restore --list "${dump}" > "${dump}.toc"
	else
		cat "${dump}" | docker compose exec -T "${A2A888_DB_SERVICE:-db}" pg_restore --list - > "${dump}.toc"
	fi
	grep -q 'TABLE.*organizations' "${dump}.toc"
	grep -q 'TABLE.*a2a888_approval_request' "${dump}.toc"
	grep -q 'TABLE.*a2a888_connector_credential' "${dump}.toc"
	grep -q 'TABLE.*a2a888_usage_event' "${dump}.toc"
	echo "backup manifest verified: ${dump}"
}

[[ "$#" -ge 2 ]] || { usage; exit 2; }
case "$1" in
	backup) backup "$2" ;;
	restore) restore "$2" ;;
	verify) verify "$2" ;;
	*) usage; exit 2 ;;
esac
