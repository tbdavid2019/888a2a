# Observability and SLO contract

Date: 2026-08-27

Every API, connector, A2A, runtime, approval, Message Plane, and outbox
operation must carry `organization_id` and `correlation_id` in structured
logs and durable trace events. Values are identifiers only; credentials,
parameters containing secrets, and model thoughts are excluded.

The initial SLO dimensions are:

- API and webhook acknowledgement latency.
- Outbox age and dead-letter rate per Organization and installation.
- Message Plane append/replay latency and reconciliation drift.
- A2A task completion, authorization-required, retry, and cancellation rates.
- Provider preparation failures and approval wait duration.

The metrics backend and dashboard deployment are environment-specific. The
application contract is implemented in `backend/observability/context.go`;
dashboard provisioning remains a production deployment task.
