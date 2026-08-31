## 1. Storage contract and selection

- [x] 1.1 Define the ObjectStore interface and backend selection rules; verify empty S3 selects local and partial S3 configuration fails clearly.
- [x] 1.2 Implement local filesystem put/get/delete with atomic writes and bounded reads.
- [x] 1.3 Verify local path traversal, symlink escape, tenant prefix, and concurrent operation behavior.

## 2. Service integration

- [x] 2.1 Route channel files through ObjectStore without changing file metadata or IAM behavior.
- [x] 2.2 Route user and Agent avatars through ObjectStore without changing avatar metadata or IAM behavior.
- [x] 2.3 Verify upload, download, delete, and rollback behavior without S3. CI covers local delete and rollback paths; the deployed Manager on `10.9.0.11` verified authenticated upload, download, and persistence after container recreation without S3.

## 3. Deployment and provider guidance

- [x] 3.1 Add Docker persistent object-data volume and local storage environment configuration.
- [x] 3.2 Update storage settings UI, README, and deployment guides with local, AWS S3, Cloudflare R2, and GCP interoperability examples.
- [x] 3.3 Add backup/restore and permissions guidance for local object data.

## 4. Validation

- [x] 4.1 Run backend, frontend, Proto, naming, OpenSpec, and production build checks.
- [x] 4.2 Push one green GitHub Actions run and record the dated CHANGELOG entry.
