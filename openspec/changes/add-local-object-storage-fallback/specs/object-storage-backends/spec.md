## Purpose

讓自架部署在沒有雲端物件儲存時仍能安全保存檔案與頭像，同時保留 AWS S3、
Cloudflare R2 與 GCP Cloud Storage interoperability 等 S3-compatible backend。

## ADDED Requirements

### Requirement: Local storage is the default backend when S3 is empty

The system SHALL use a configured local object-storage directory when both S3
endpoint and bucket are empty. File and avatar upload, download, and delete
operations SHALL remain available without S3 credentials.

#### Scenario: Upload without S3 configuration

- **WHEN** an authenticated caller uploads a valid file and S3 endpoint and
  bucket are empty
- **THEN** the object is stored below the local object-storage directory and
  the normal file metadata response is returned

#### Scenario: Docker restart preserves local objects

- **WHEN** the Manager container is recreated with the Compose object-data
  volume attached
- **THEN** previously stored local objects remain downloadable

### Requirement: Local object paths are tenant-safe

The local backend SHALL enforce tenant-prefixed object keys and SHALL reject
absolute paths, path traversal, NUL bytes, and symlink traversal outside the
configured root.

#### Scenario: Unsafe key is rejected

- **WHEN** a local object operation receives a key containing `..`, an absolute
  path, or a symlink that resolves outside the root
- **THEN** the operation fails without reading, writing, deleting, or following
  the outside path

### Requirement: S3-compatible backends remain selectable

The system SHALL continue to use S3-compatible storage when endpoint and bucket
are configured completely. The documentation SHALL cover AWS S3, Cloudflare
R2, and GCP Cloud Storage interoperability settings.

#### Scenario: R2 or GCS interoperability is configured

- **WHEN** an operator supplies a valid S3-compatible endpoint, bucket, and
  credentials
- **THEN** object operations use that remote backend and do not write local
  object files
