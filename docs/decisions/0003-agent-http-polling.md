# 0003: Agent HTTP Polling Before gRPC

Status: accepted for MVP

Kronos agents use outbound HTTP polling against the control plane for
heartbeat, job claim, job finish, and evidence-reporting workflows. gRPC remains
roadmap work rather than an MVP requirement.

## Rationale

HTTP polling fits the current single-replica control plane and keeps the worker
deployment model simple:

- Agents only need outbound network access to the control plane.
- The same bearer-token, request-ID, TLS, and optional mTLS configuration covers
  CLI, agent, WebUI, and automation clients.
- The OpenAPI contract and existing server tests exercise the production API
  surface directly.
- Queued jobs remain durable in the control-plane state store; workers can
  reconnect and claim work without a long-lived bidirectional stream.

The tradeoff is latency and efficiency. Polling is less efficient than a
push-based stream for large fleets, and it does not provide native backpressure
or streaming progress semantics.

## Requirements

- Agents must send regular heartbeats with capacity and labels.
- The control plane must fail or recover stale running jobs after server or
  agent loss.
- Job claim and finish endpoints must remain idempotent enough for retrying
  transient network failures.
- Request IDs must be propagated so failed claims and finishes are traceable in
  logs and client output.

## Decision Gate

For v0.1/MVP:

- Keep HTTP polling as the supported agent transport.
- Document gRPC as future scale/streaming work.
- Harden polling behavior with stale-agent recovery, retry guidance, and
  operator troubleshooting docs.

For a larger multi-tenant or high-agent-count release:

- Re-evaluate gRPC or another streaming transport if polling latency, request
  volume, or progress-reporting gaps become material.
