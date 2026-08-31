## Context

The current service methods call the AWS SDK directly and translate an empty
S3 setting into a failed precondition. The replacement keeps the service
boundaries stable and moves backend selection into one object-storage
component.

## Decisions

1. `ObjectStore` is the narrow interface used by file and avatar services:
   put, get, and delete. The S3 implementation wraps the AWS SDK; the local
   implementation uses regular files.
2. Local storage is selected only when both S3 endpoint and bucket are empty.
   A partial S3 configuration fails clearly instead of silently writing to a
   different backend.
3. `A2A888_OBJECT_STORAGE_DIR` configures the local root. Source runs default
   to `./data/objects`; Docker Compose sets `/data/objects` and mounts the
   persistent `objectdata` volume.
4. Local paths are derived from already tenant-prefixed object keys. The local
   backend rejects absolute paths, `..`, NUL bytes, and symlink traversal. It
   writes to a same-directory temporary file, syncs it, and atomically renames
   it into place.
5. Cloudflare R2 uses its S3-compatible endpoint. GCP Cloud Storage can use
   its S3 interoperability endpoint and HMAC credentials. Native GCS service
   account support remains a separate future adapter.

## Compatibility

Existing S3 settings and object keys remain valid. Existing closed-mode IAM,
organization prefixes, and database file rows remain unchanged. A local object
backend does not expose a new unauthenticated HTTP file path.

## Failure and Recovery

Storage errors return the existing internal error contract. Database metadata
is committed only after a successful object write. Local object files are
recoverable from the Docker volume; operators must back up that volume together
with PostgreSQL.
