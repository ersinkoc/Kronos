# Kronos Control Plane API

Generated from [`api/openapi/openapi.yaml`](../api/openapi/openapi.yaml). Do not edit endpoint tables by hand; update the OpenAPI spec and rerun `KRONOS_UPDATE_API_DOCS=1 go test ./api/openapi`.

Version: `0.1.0`

Local/no-token mode accepts requests without Authorization. When a bearer token is provided, endpoints enforce exact scopes plus resource:*, admin:*, or *. Clients may send X-Kronos-Request-ID for request correlation; the control plane echoes it on every response or generates one when omitted. Health, readiness, metrics, and overview endpoints support GET and HEAD for probe-friendly status checks. Control-plane API, health, readiness, and metrics responses are sent with `Cache-Control: no-store`, and API/WebUI responses include baseline browser security headers such as X-Content-Type-Options, X-Frame-Options, Referrer-Policy, Permissions-Policy, and Content-Security-Policy.

## Server

- `http://127.0.0.1:8500`

## Endpoints

| Method | Path | Summary | Success responses |
| --- | --- | --- | --- |
| `GET` | `/api/v1/agents` | List agents | 200 |
| `POST` | `/api/v1/agents/heartbeat` | Record agent heartbeat | 200 |
| `GET` | `/api/v1/agents/{id}` | Get agent | 200 |
| `GET` | `/api/v1/audit` | List audit events | 200 |
| `POST` | `/api/v1/audit/verify` | Verify audit hash chain | 200 |
| `POST` | `/api/v1/auth/verify` | Verify bearer API token | 200 |
| `GET` | `/api/v1/backups` | List backups | 200 |
| `POST` | `/api/v1/backups/now` | Enqueue an immediate backup | 200 |
| `GET` | `/api/v1/backups/{id}` | Inspect a backup | 200 |
| `POST` | `/api/v1/backups/{id}/protect` | Enable manual protection | 200 |
| `POST` | `/api/v1/backups/{id}/unprotect` | Disable manual protection | 200 |
| `POST` | `/api/v1/backups/{id}/verify` | Enqueue backup verification | 200 |
| `GET` | `/api/v1/jobs` | List jobs | 200 |
| `POST` | `/api/v1/jobs/claim` | Claim the oldest queued job | 200 |
| `GET` | `/api/v1/jobs/{id}` | Inspect a job | 200 |
| `POST` | `/api/v1/jobs/{id}/cancel` | Cancel a queued or running job | 200 |
| `GET` | `/api/v1/jobs/{id}/evidence` | Export a job evidence artifact | 200 |
| `POST` | `/api/v1/jobs/{id}/finish` | Finish a job and optionally capture backup metadata | 200 |
| `POST` | `/api/v1/jobs/{id}/retry` | Requeue a failed or canceled job | 200 |
| `GET` | `/api/v1/notifications` | List notification rules | 200 |
| `POST` | `/api/v1/notifications` | Create notification rule | 200 |
| `GET` | `/api/v1/notifications/{id}` | Get notification rule | 200 |
| `PUT` | `/api/v1/notifications/{id}` | Update notification rule | 200 |
| `DELETE` | `/api/v1/notifications/{id}` | Delete notification rule | 204 |
| `GET` | `/api/v1/overview` | Operations overview | 200 |
| `HEAD` | `/api/v1/overview` | Operations overview headers | 200 |
| `POST` | `/api/v1/restore` | Enqueue a restore job | 200 |
| `POST` | `/api/v1/restore/preview` | Preview restore chain and target | 200 |
| `POST` | `/api/v1/retention/apply` | Apply retention decisions to backup metadata | 200 |
| `POST` | `/api/v1/retention/plan` | Preview retention decisions | 200 |
| `GET` | `/api/v1/retention/policies` | List retention policies | 200 |
| `POST` | `/api/v1/retention/policies` | Create retention policy | 200 |
| `GET` | `/api/v1/retention/policies/{id}` | Get retention policy | 200 |
| `PUT` | `/api/v1/retention/policies/{id}` | Update retention policy | 200 |
| `DELETE` | `/api/v1/retention/policies/{id}` | Delete retention policy | 204 |
| `POST` | `/api/v1/scheduler/tick` | Evaluate schedules once and enqueue due jobs | 200 |
| `GET` | `/api/v1/schedules` | List schedules | 200 |
| `POST` | `/api/v1/schedules` | Create schedule | 200 |
| `GET` | `/api/v1/schedules/{id}` | Get schedule | 200 |
| `PUT` | `/api/v1/schedules/{id}` | Update schedule | 200 |
| `DELETE` | `/api/v1/schedules/{id}` | Delete schedule | 204 |
| `POST` | `/api/v1/schedules/{id}/pause` | Pause schedule | 200 |
| `POST` | `/api/v1/schedules/{id}/resume` | Resume schedule | 200 |
| `GET` | `/api/v1/storages` | List storages | 200 |
| `POST` | `/api/v1/storages` | Create storage | 200 |
| `GET` | `/api/v1/storages/{id}` | Get storage | 200 |
| `PUT` | `/api/v1/storages/{id}` | Update storage | 200 |
| `DELETE` | `/api/v1/storages/{id}` | Delete storage | 204 |
| `GET` | `/api/v1/targets` | List targets | 200 |
| `POST` | `/api/v1/targets` | Create target | 200 |
| `GET` | `/api/v1/targets/{id}` | Get target | 200 |
| `PUT` | `/api/v1/targets/{id}` | Update target | 200 |
| `DELETE` | `/api/v1/targets/{id}` | Delete target | 204 |
| `GET` | `/api/v1/tokens` | List API tokens | 200 |
| `POST` | `/api/v1/tokens` | Create API token | 200 |
| `POST` | `/api/v1/tokens/prune` | Prune revoked and expired API tokens | 200 |
| `GET` | `/api/v1/tokens/{id}` | Get API token | 200 |
| `POST` | `/api/v1/tokens/{id}/revoke` | Revoke API token | 200 |
| `GET` | `/api/v1/users` | List users | 200 |
| `POST` | `/api/v1/users` | Create user | 200 |
| `GET` | `/api/v1/users/{id}` | Get user | 200 |
| `DELETE` | `/api/v1/users/{id}` | Delete user | 204 |
| `POST` | `/api/v1/users/{id}/grant` | Grant user role | 200 |
| `GET` | `/healthz` | Health check | 200 |
| `HEAD` | `/healthz` | Health check headers | 200 |
| `GET` | `/metrics` | Prometheus metrics | 200 |
| `HEAD` | `/metrics` | Prometheus metrics headers | 200 |
| `GET` | `/readyz` | Readiness check | 200 |
| `HEAD` | `/readyz` | Readiness check headers | 200 |

## Schemas

### AgentHeartbeat

- Type: `object`
- Required: `id`
- Properties: `address`, `capacity`, `id`, `labels`, `version`

### AgentSnapshot

- Type: `object`
- Required: `id`, `last_heartbeat`, `status`
- Properties: `address`, `capacity`, `id`, `labels`, `last_heartbeat`, `status`, `version`

### AgentsResponse

- Type: `object`
- Required: `agents`
- Properties: `agents`

### AuditEvent

- Type: `object`
- Required: `action`, `hash`, `id`, `occurred_at`, `resource_type`, `seq`
- Properties: `action`, `actor_id`, `hash`, `id`, `metadata`, `occurred_at`, `prev_hash`, `resource_id`, `resource_type`, `seq`

### AuditEventsResponse

- Type: `object`
- Required: `events`
- Properties: `events`

### AuthVerifyResponse

- Type: `object`
- Required: `token`
- Properties: `token`

### Backup

- Type: `object`
- Required: `id`, `manifest_id`
- Properties: `chunk_count`, `ended_at`, `id`, `job_id`, `manifest_id`, `parent_id`, `protected`, `size_bytes`, `started_at`, `storage_id`, `target_id`, `type`

### BackupNowRequest

- Type: `object`
- Required: `storage_id`, `target_id`
- Properties: `parent_id`, `storage_id`, `target_id`, `type`

### BackupVerifyRequest

- Type: `object`
- Properties: `level`

### BackupsResponse

- Type: `object`
- Required: `backups`
- Properties: `backups`

### ClaimJobResponse

- Type: `object`
- Properties: `job`

### CreateTokenRequest

- Type: `object`
- Required: `name`, `scopes`, `user_id`
- Properties: `expires_at`, `name`, `scopes`, `user_id`

### CreatedTokenResponse

- Type: `object`
- Required: `secret`, `token`
- Properties: `secret`, `token`

### ErrorResponse

- Type: `object`
- Required: `error`
- Properties: `error`

### EvidenceArtifact

- Type: `object`
- Properties: `created_at`, `id`, `job_id`, `kind`, `restore`, `sha256`

### FailureEvidence

- Type: `object`
- Properties: `at`, `backup_id`, `manifest_ids`, `message`, `operation`, `stage`, `storage_id`, `target_id`

### FinishJobRequest

- Type: `object`
- Required: `status`
- Properties: `backup`, `error`, `failure`, `restore`, `status`, `verification`

### HealthResponse

- Type: `object`
- Required: `status`
- Properties: `projects`, `status`

### Job

- Type: `object`
- Required: `id`, `queued_at`, `status`
- Properties: `agent_id`, `ended_at`, `error`, `evidence_artifact`, `failure_evidence`, `id`, `operation`, `parent_backup_id`, `queued_at`, `restore_at`, `restore_backup_id`, `restore_dry_run`, `restore_manifest_id`, `restore_manifest_ids`, `restore_replace_existing`, `restore_report`, `restore_target_id`, `schedule_id`, `started_at`, `status`, `storage_id`, `target_id`, `type`, `verify_backup_id`, `verify_level`, `verify_manifest_id`, `verify_manifest_ids`, `verify_report`

### JobsResponse

- Type: `object`
- Required: `jobs`
- Properties: `jobs`

### NotificationRule

- Type: `object`
- Required: `created_at`, `enabled`, `events`, `id`, `name`, `updated_at`, `webhook_url`
- Properties: `created_at`, `enabled`, `events`, `id`, `max_attempts`, `name`, `secret`, `updated_at`, `webhook_url`

### NotificationRulesResponse

- Type: `object`
- Required: `notifications`
- Properties: `notifications`

### OKResponse

- Type: `object`
- Required: `ok`
- Properties: `ok`

### OverviewResponse

- Type: `object`
- Required: `agents`, `attention`, `backups`, `generated_at`, `health`, `inventory`, `jobs`
- Properties: `agents`, `attention`, `backups`, `generated_at`, `health`, `inventory`, `jobs`, `latest_backups`, `latest_jobs`

### PruneTokensRequest

- Type: `object`
- Properties: `dry_run`

### PruneTokensResponse

- Type: `object`
- Required: `deleted`, `dry_run`, `tokens`
- Properties: `deleted`, `dry_run`, `tokens`

### ReadinessResponse

- Type: `object`
- Required: `checks`, `status`
- Properties: `checks`, `error`, `status`

### RestoreEvidence

- Type: `object`
- Properties: `backup_id`, `dry_run`, `ended_at`, `error`, `failure`, `manifest_ids`, `operation`, `queued_at`, `replace_existing`, `report`, `restore_at`, `started_at`, `status`, `storage_id`, `target_id`

### RestorePlan

- Type: `object`
- Required: `backup_id`, `steps`, `storage_id`, `target_id`
- Properties: `at`, `backup_id`, `steps`, `storage_id`, `target_id`, `warnings`

### RestorePreviewRequest

- Type: `object`
- Required: `backup_id`
- Properties: `at`, `backup_id`, `target_id`

### RestoreReport

- Type: `object`
- Properties: `backup_id`, `chunks`, `dry_run`, `manifest_ids`, `objects`, `restored_bytes`, `stored_bytes`, `target_id`

### RestoreStartRequest

- Composes: `RestorePreviewRequest`
- Additional properties: `dry_run`, `replace_existing`

### RestoreStartResponse

- Type: `object`
- Required: `job`, `plan`
- Properties: `job`, `plan`

### RetentionApplyRequest

- Composes: `RetentionPlanRequest`
- Additional properties: `dry_run`

### RetentionApplyResponse

- Type: `object`
- Required: `deleted`, `dry_run`, `plan`
- Properties: `deleted`, `dry_run`, `plan`

### RetentionPlan

- Type: `object`
- Required: `items`
- Properties: `items`

### RetentionPlanItem

- Type: `object`
- Required: `backup`, `keep`
- Properties: `backup`, `keep`, `reasons`

### RetentionPlanRequest

- Type: `object`
- Required: `policy`
- Properties: `now`, `policy`

### RetentionPoliciesResponse

- Type: `object`
- Required: `policies`
- Properties: `policies`

### RetentionPolicy

- Type: `object`
- Required: `created_at`, `id`, `name`, `rules`, `updated_at`
- Properties: `created_at`, `id`, `name`, `rules`, `updated_at`

### RetentionRule

- Type: `object`
- Required: `kind`
- Properties: `kind`, `params`

### Schedule

- Type: `object`
- Required: `backup_type`, `created_at`, `expression`, `id`, `name`, `paused`, `storage_id`, `target_id`, `updated_at`
- Properties: `backup_type`, `created_at`, `expression`, `id`, `labels`, `name`, `paused`, `retention_policy_id`, `storage_id`, `target_id`, `updated_at`

### SchedulesResponse

- Type: `object`
- Required: `schedules`
- Properties: `schedules`

### Storage

- Type: `object`
- Required: `id`, `kind`, `name`, `uri`
- Properties: `created_at`, `id`, `kind`, `labels`, `name`, `options`, `updated_at`, `uri`

### StoragesResponse

- Type: `object`
- Required: `storages`
- Properties: `storages`

### Target

- Type: `object`
- Required: `driver`, `endpoint`, `id`, `name`
- Properties: `created_at`, `database`, `driver`, `endpoint`, `id`, `labels`, `name`, `options`, `updated_at`

### TargetsResponse

- Type: `object`
- Required: `targets`
- Properties: `targets`

### Token

- Type: `object`
- Required: `created_at`, `id`, `name`, `scopes`, `user_id`
- Properties: `created_at`, `expires_at`, `id`, `name`, `revoked_at`, `scopes`, `user_id`

### TokensResponse

- Type: `object`
- Required: `tokens`
- Properties: `tokens`

### User

- Type: `object`
- Required: `display_name`, `email`, `role`
- Properties: `created_at`, `display_name`, `email`, `id`, `role`, `totp_enforced`, `updated_at`

### UsersResponse

- Type: `object`
- Required: `users`
- Properties: `users`

### VerificationReport

- Type: `object`
- Properties: `backup_id`, `chunks`, `level`, `manifest_ids`, `objects`, `restored_bytes`, `stored_bytes`, `verified_chunks`

