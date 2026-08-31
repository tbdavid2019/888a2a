## Why

File and avatar upload/download currently stop when an administrator has not
configured S3. This makes a Docker-only self-hosted installation needlessly
incomplete even though the Manager already has a durable filesystem available.

## What Changes

- Add a local filesystem object-storage backend for file and avatar upload,
  download, and delete operations.
- Select local storage automatically when the S3 setting is empty; keep S3 and
  S3-compatible services as the preferred backend when configured completely.
- Mount a persistent Docker object-data volume and document its backup and
  permissions.
- Document AWS S3, Cloudflare R2, and GCP Cloud Storage interoperability
  configuration.
- Preserve tenant-prefixed object keys and reject path traversal, symlinks, and
  unsafe object names in local storage.

## Capabilities

### New Capabilities

- `object-storage-backends`: Select and operate local or S3-compatible object
  storage with the same upload/download contract.

### Modified Capabilities

- `organization-tenancy`: Tenant isolation now applies to local object keys and
  filesystem paths as well as S3 object keys.

## Impact

- Affected Go storage component, file/avatar services, Docker Compose, runtime
  configuration, settings UI copy, tests, and deployment documentation.
- No new cloud dependency is required for local storage, R2, or GCS S3
  interoperability.
