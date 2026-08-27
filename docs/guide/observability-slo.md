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

The Manager now propagates a bounded `X-Correlation-ID` (or generates one),
adds it to the response header, and carries it with the tenant context into
Connect handlers and audit logs. Durable outbox and A2A trace records retain
their own correlation fields. The metrics backend and dashboard deployment
remain environment-specific production tasks; do not use untrusted tenant
values as Prometheus labels.

For a request-level trace, send `X-Correlation-ID: <short-id>` and search the
structured Manager logs, audit records, outbox events, and A2A trace events for
that ID. Credentials, raw payloads, and model thoughts must not be added to
the correlation metadata.

The `/metrics` endpoint also exports tenant-scoped operation counters and
latency histograms:

- `a2a888_operation_requests_total{organization_id,surface,operation,status}`
- `a2a888_operation_duration_seconds{organization_id,surface,operation}`

Configure Prometheus and Grafana retention according to the deployment's
privacy policy. Do not expose `/metrics` to public clients.
