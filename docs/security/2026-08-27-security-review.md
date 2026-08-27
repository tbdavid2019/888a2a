# 888a2a security review

Date: 2026-08-27

Scope: organization tenancy, connector ingress/egress, credential storage,
runtime execution, A2A work, approval binding, file handling, and durable
event paths implemented in this change.

## Evidence reviewed

- Organization IAM resolves tenant scope before resource lookup and rejects
  inactive organizations and non-active memberships.
- LINE verification uses HMAC-SHA256 over the exact raw body and compares with
  `hmac.Equal` before normalization or inbox persistence.
- Connector credentials use AES-GCM with associated data containing both the
  organization and installation identity. Responses and logs do not include
  plaintext credentials.
- Runtime paths are confined to per-Agent roots; unclassified and dangerous
  ACP actions fail closed or wait for a bound approval decision.
- Approval requests bind organization, workspace, Agent, action parameters,
  intent hash, expiry, and a single-use nonce. Decision persistence is atomic.
- Durable inbox/outbox identities are tenant-scoped and idempotent. Unsupported
  connector operations produce explicit divergence records.

## Release blockers

The following surfaces remain disabled or pending and are not represented as
security-complete:

- Real LINE channel credentials and Console webhook canary.
- Full WuKongIM failover, backup, restore, and offline-sync evidence.
- Slack, Teams, and WhatsApp OAuth/provider review and production adapters.
- Repository-wide removal of legacy compatibility identifiers after the
  migration window.

These are release-readiness blockers, not silently accepted high-risk findings.
The implemented paths have no open critical/high finding from this review.

## Verification

`go vet ./backend/...`, `go test ./backend/...`, frontend lint/type/tests/build,
Proto lint, naming gate, and the controlled PostgreSQL tenancy, approval,
credential, connector, and usage gates passed for the reviewed code.
