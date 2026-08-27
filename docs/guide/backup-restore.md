# 888a2a Backup and Disaster-Recovery Drill

Date: 2026-08-27

The Manager database is the system of record for tenant, task, approval,
connector, usage, audit, and projection data. Backups must be encrypted and
stored outside the host. The commands below use PostgreSQL custom-format dumps
and never put a database password in the repository.

## Backup

```bash
export A2A888_PG_URL='postgres://user:password@db-host:5432/888a2a?sslmode=require'
scripts/backup_888a2a.sh backup /secure/backup/888a2a
```

The command writes a custom-format dump, schema dump, table-count snapshot,
and SHA-256 sidecars. Record the backup timestamp and the observed RPO.

## Verify a backup without restoring it

```bash
scripts/backup_888a2a.sh verify /secure/backup/888a2a
```

Verification checks the checksum and confirms that the dump contains the
Organization, approval, connector-credential, and usage tables.

## Restore into an isolated database

```bash
export A2A888_PG_URL='postgres://user:password@drill-db:5432/888a2a_restore?sslmode=require'
A2A888_RESTORE_CONFIRM=YES scripts/backup_888a2a.sh restore \
  /secure/backup/888a2a/888a2a-<timestamp>.dump
```

The restore command is intentionally destructive for its target database.
Use a fresh isolated database for every drill. After restoring, run the
Manager migration verifier and compare tenant counts, message/task sequence
high-watermarks, approval states, connector installation identities,
credential key versions, and artifact references with the backup snapshot.

## Declared drill targets

Before accepting a production drill, record:

- RPO: maximum accepted data loss, measured from the last successful dump.
- RTO: time from declaring a failure to a healthy Manager serving the restored
  tenant data.
- Restore verification output and the operator who approved the result.
- Rollback: keep the old Manager and original database untouched until the
  restored instance passes health, authentication, and tenant-isolation checks.

The automated command and manifest checks are available now. A production
RPO/RTO drill remains an environment-specific acceptance gate until it is run
against the controlled deployment.
