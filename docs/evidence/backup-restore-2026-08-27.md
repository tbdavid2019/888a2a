# Backup and Restore Drill Evidence

Date: 2026-08-27

## Environment

- Host: controlled Docker VM `10.9.0.11`
- Source: the isolated `888a2a` PostgreSQL Compose service
- Target: fresh database `a2a888_restore_20260827` in the same PostgreSQL container
- Method: `scripts/backup_888a2a.sh backup`, `verify`, and `restore`

## Results

The custom-format dump checksum and table-of-contents verification passed.
The following counts were captured before and after restore, in this order:

`organizations | principal | organization_memberships | machine | approval_requests | connector_installations | usage_events | messages`

```text
before=1|2|2|1|0|0|0|0
after=1|2|2|1|0|0|0|0
restore_seconds=36
```

The target database was created from scratch and the original database was
left untouched. The temporary target was removed after verification.

## Scope limitation

This is an isolated PostgreSQL restore drill. It does not prove WuKongIM
cluster failover, object-storage recovery, multi-node Manager recovery, or a
production RPO/RTO target. Those acceptance items remain pending until the
controlled production topology and declared RPO/RTO window are available.
