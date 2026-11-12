# Authorization Service Specification

## Purpose

Provide centralized authorization capabilities through OpenFGA integration.

The bot is the currently supported service client.

The bot serves as a personal assistant for notes and reminders management in personal and shared spaces.

Admin clients are planned and are not supported by the current OpenFGA model.

## Implemented APIs

The current service provides:

1. Service-token issuance using client credentials.
2. Resource and relationship synchronization to OpenFGA.
3. Single-action authorization through `POST /v0/decisions/check`.

- OpenFGA is the authoritative relationship and authorization model for spaces, notes, reminders, and members.
- The current implemented resource synchronization API writes relationships to OpenFGA.
- The single-check API follows the contract below; unfinished legacy check routes are not public aliases.

## Architecture Overview

- Telegram sends an update to the bot.
- The bot extracts Telegram user identity from the verified Telegram update.
- The bot sends the request to the web server with `Authorization: Bearer <bot-token>` and `X-Telegram-User-Id`.
- The web server forwards the request to the authorization service.
- The web server acts as a transport proxy for bot authorization requests.
- The web server accepts `X-Telegram-User-Id` only from an authenticated bot request.
- The web server MUST NOT trust, use, or forward an `X-Telegram-User-Id` supplied by an external HTTP client.
- The authorization service validates the bot token forwarded by the web server before using `X-Telegram-User-Id`.
- The authorization service maps Telegram user ID from `X-Telegram-User-Id` to an internal user ID in the application database.
- The current resource synchronization API writes resource relationships to OpenFGA.
- The authorization check API SHALL query OpenFGA using the internal user ID, the mapped relation, and the target object, except space:create, which is permitted to every registered user.
- The authorization check API SHALL return an `allow` or `deny` decision, constrained by the field policy for update actions.
- Tokens are issued to service clients; the bot is the currently supported service client.
- Telegram end users do not receive service tokens.
- The OpenFGA model is defined in `./fga/auth-model.fga`.

## Domain Model

### Spaces

- **Personal space**: Private to a single user and created as the user's default space.
- **Public space**: Shared among multiple members through explicit access grants.

### Space Roles

The OpenFGA model defines the following space roles:

- `owner`: Can manage the space and inherits admin permissions.
- `admin`: Can manage members and inherits editor permissions.
- `editor`: Can create notes and reminders and inherits viewer permissions.
- `viewer`: Can view accessible notes and reminders.

### Action Categories

The following actions belong to the single-check API.

**Spaces:** create/read/update/delete; personal space deletion is always forbidden.

**Notes:**

- `note:create` — create a new note in a space.
- `note:read` — view note content.
- `note:update` — edit an existing note.
- `note:delete` — permanently delete a note.

**Reminders:**

- `reminder:create` — create a new reminder in a space.
- `reminder:read` — view reminder details.
- `reminder:update` — edit an existing reminder.
- `reminder:stop` — stop an active reminder without deleting it.
- `reminder:delete` — permanently delete a reminder.

**Members:**

- `member:add` — add a user to a space with role `viewer`, `editor`, or `admin`.
- `member:remove` — remove a user from a space.
- `member:edit` — change a user's role in a space to `viewer`, `editor`, or `admin`.

### Deletion Semantics

The following rules apply to deletion checks. Direct ownership of a record in a personal space grants no rights to a user other than that space owner. Personal spaces themselves cannot be deleted, including by their owner.

**Notes:**

- The owner of a note can delete the note.
- The owner of the space containing the note can delete any note in that space.
- Admins and editors cannot delete a note unless they are the note owner.

**Reminders:**

- The owner of a reminder can delete the reminder.
- The owner of the space containing the reminder can delete any reminder in that space.
- Admins and editors cannot delete a reminder unless they are the reminder owner.

These rules are enforced by the OpenFGA model via the `can_delete` relation on `note` and `reminder` objects.

## Authorization Technology Boundaries

OpenFGA SHALL be the source of truth for authorization of spaces, notes, reminders, and members.

Casbin SHALL constrain update fields after OpenFGA permits the action. The embedded model and policy are immutable per build and contain no user-role assignments. The package `internal/service/enforcer` owns this field policy; legacy `internal/service/politics` SHALL NOT be used by new authorization code.

Allowed update fields are note: `text,title`, reminder: `remind_at,title`, space: `name`. These lists apply equally to all authorized users; all other fields are forbidden, including technical fields. The reminder list is provisional for the future data model. Policy changes require a new build and change `policy_version`.

Personal/shared types are trusted OpenFGA metadata. Exactly one type is required; missing or conflicting types grant no access. Personal spaces and their contents are accessible only to their owner, cannot have members, and cannot be deleted. Shared spaces retain the owner/admin/editor/viewer hierarchy. The owner may update the name of either type; only shared spaces can be deleted. Deleting a personal-space record remains permitted to the space owner.

## Service Token Issuance

### Login Request

The service SHALL accept a login request containing:

| Field | Required | Rules |
|---|---:|---|
| `grant_type` | Yes | Only `client_credentials` is supported |
| `client_id` | Yes | Identifies an active service client |
| `client_secret` | Yes | Validated against a secret obtained from Vault |
| `scope` | No | Space-separated requested scopes |

### Login Response

On successful authentication, the service SHALL return:

| Field | Rules |
|---|---|
| `access_token` | Signed JWT |
| `token_type` | Always `Bearer` |
| `expires_in` | Access token TTL in seconds |
| `scope` | Effective space-separated scope list |

### Token Requirements

REQ-TOKEN-001: The service SHALL support only `grant_type=client_credentials`.

REQ-TOKEN-002: The service SHALL issue tokens only to active service clients.

REQ-TOKEN-003: The service SHALL validate `client_secret` without logging it.

REQ-TOKEN-004: If `scope` is omitted, the service SHALL issue the full scope list assigned to the service client.

REQ-TOKEN-005: If `scope` is provided, every requested scope SHALL be included in the service client's allowed scopes.

REQ-TOKEN-006: If a requested scope is not allowed, the service SHALL reject the request.

REQ-TOKEN-007: The successful response SHALL contain a signed JWT access token, `token_type="Bearer"`, token TTL, and effective scopes.

REQ-TOKEN-008: The service SHALL NOT issue service tokens to Telegram end users.

REQ-TOKEN-009: The service SHALL obtain client secrets from Vault and SHALL NOT read secrets from `.env` files or version control.

REQ-TOKEN-010: The access token TTL SHALL be 3600 seconds.

REQ-TOKEN-011: The service SHALL return the token TTL in the `expires_in` field of the login response.

## Resource Synchronization API

The service SHALL accept `UpdateResourceRequest` to synchronize resource relationships with OpenFGA.

### Supported Resources

- `space`
- `note`
- `reminder`
- `user`

### Supported Operations

- `create`
- `update`
- `delete`

### Supported Change Types

| Change type | Required operation |
|---|---|
| `resource_added` | `create` |
| `resource_removed` | `delete` |
| `resource_moved` | `update` |
| `membership_added` | `create` |
| `membership_changed` | `update` |
| `membership_removed` | `delete` |

### Request Fields

`UpdateResourceRequest` SHALL contain:

- `request_id`;
- `idempotency_key`;
- `decision_id`;
- `resource`;
- `relations`;
- `operation`;
- `change_type`;
- `context`.

`TelegramID` is transport metadata derived from `X-Telegram-User-Id` and SHALL NOT be serialized into the JSON request body.

The service provides a single universal `UpdateResource` endpoint for all resource types.

The service SHALL NOT provide separate endpoints for notes, reminders, users, or other resource types.

All resource synchronization operations SHALL use `UpdateResourceRequest` with the appropriate `resource.type`, `operation`, `change_type`, and `relations`.

### Resource Sync Requirements

REQ-SYNC-001: The service SHALL reject a request whose `operation` does not match `change_type`.

REQ-SYNC-002: The service SHALL require non-empty valid UUID values for `request_id` and `decision_id`.

REQ-SYNC-003: The service SHALL require a non-empty `idempotency_key`.

REQ-SYNC-004: The service SHALL require `resource`, `operation`, `change_type`, and `context`.

REQ-SYNC-005: The service SHALL use `decision_id` as the identifier of the authorization decision that permitted the resource synchronization.

REQ-SYNC-006: The service SHALL persist `decision_id` with the idempotency record and include it in audit data.

REQ-SYNC-007: The service SHALL use `request_id` as the request and audit correlation identifier.

REQ-SYNC-008: `decision_id` SHALL reference an authorization decision produced by an approved trusted authorization component.

REQ-SYNC-009: The service SHALL validate that `decision_id` is present and well-formed, but SHALL NOT infer authorization from `decision_id` alone unless decision verification is explicitly implemented.

REQ-SYNC-010: Until decision verification is implemented, the service SHALL accept `decision_id` as trusted metadata from the authenticated service client and SHALL NOT treat it as an authorization proof.

REQ-SYNC-011: The service SHALL authenticate the calling service client before accepting `UpdateResourceRequest`.

REQ-SYNC-012: A note or reminder resource SHALL define a parent `space`.

REQ-SYNC-013: A created note or reminder SHALL define an owner user.

REQ-SYNC-014: The service SHALL translate a note parent relation to `note:<id>#space@space:<id>`.

REQ-SYNC-015: The service SHALL translate a reminder parent relation to `reminder:<id>#space@space:<id>`.

REQ-SYNC-016: The service SHALL translate an owner relation to `<resource-type>:<resource-id>#owner@user:<user-id>`.

REQ-SYNC-017: The service SHALL return written and deleted tuples in `UpdateResourceResponse`.

REQ-SYNC-018: The service SHALL return the OpenFGA authorization model ID in `meta.auth_model_id`.

REQ-SYNC-019: The service SHALL use `context.source_service`, `context.event_type`, and `context.trace_id` for traceability and logging.

REQ-SYNC-020: The service SHALL not log secrets from incoming headers or token values.

## UpdateResource Idempotency (separate future task)

The requirements in this section are a future contract, not implemented behavior. By explicit BZ-167 scope decision, reservation, replay, TTL, fingerprint storage and recovery are deferred to a separate task. The existing key remains request/response metadata; BZ-167 SHALL NOT claim these guarantees.


### Idempotency Scope

The idempotency scope SHALL be the authenticated service client identity plus `idempotency_key`.

Two different service clients MAY use the same `idempotency_key` without conflict.

The same service client SHALL NOT be able to use one `idempotency_key` for different payloads.

### Canonical Fingerprint

Before reserving an idempotency key, the service SHALL calculate a canonical fingerprint of the complete semantic request payload.

The fingerprint SHALL include:

- `decision_id`;
- `resource`;
- `relations`;
- `operation`;
- `change_type`;
- `context`.

The fingerprint SHALL exclude:

- transport-only `TelegramID`;
- authentication tokens;
- non-semantic HTTP headers.

Canonicalization SHALL use deterministic JSON encoding with stable object-key ordering and normalized representations for UUIDs and enum values.

The service SHALL store the fingerprint and SHALL compare it on every retry.

### Reservation and Ownership

The service SHALL atomically reserve the idempotency key in the scope of the authenticated service client.

The reservation record SHALL contain at least:

- service-client identity;
- `idempotency_key`;
- canonical payload fingerprint;
- `request_id`;
- `decision_id`;
- processing status;
- creation timestamp;
- expiration timestamp;
- complete response when processing succeeds.

The request that successfully creates the reservation is the owner of the idempotency key.

Only the owner of the reservation SHALL execute the corresponding OpenFGA write.

A concurrent request that does not own an active reservation SHALL return HTTP 409 Conflict with `error_code="idempotency_in_progress"`.

It SHALL NOT wait for the owner and SHALL NOT execute OpenFGA.

The reservation owner remains responsible for completing the operation.

### Replay

An identical retry is a request with the same idempotency scope and the same canonical fingerprint.

If an identical retry finds a completed reservation, the service SHALL return the stored `UpdateResourceResponse`.

The replay SHALL return the stored response before creating an audit record and before executing an OpenFGA write.

An identical retry SHALL NOT create a new audit record.

An identical retry SHALL NOT call OpenFGA.

### Conflict

If a request uses an existing idempotency scope and key with a different fingerprint, the service SHALL return HTTP 409 Conflict.

The conflict response SHALL use the Error Response Contract with:

```json
{
  "error_code": "idempotency_conflict"
}
```

The conflict response SHALL NOT execute an OpenFGA write.

The conflict response SHALL NOT create an authorization audit record for the rejected duplicate payload.

### Completion and Failure

The owner SHALL execute the OpenFGA write only after successful reservation.

After a successful OpenFGA write, the owner SHALL persist the complete `UpdateResourceResponse` and mark the reservation as completed.

The persisted response SHALL include all fields returned to the original caller, including `request_id`, `idempotency_key`, `status`, `result`, `resource`, `written_tuples`, `deleted_tuples`, and `meta`.

If OpenFGA fails, the owner SHALL mark the reservation as failed according to the configured retry policy.

A failed reservation SHALL NOT expose a successful response.

The service SHALL define whether failed reservations are retryable. A retryable failure MAY be retried by the reservation owner or a recovery worker, but another request SHALL NOT perform an independent write without taking ownership according to the reservation state machine.

The service SHALL NOT claim database/OpenFGA atomicity. Recovery from a failure between the OpenFGA write and response persistence SHALL be handled by a transactional outbox, reconciliation job, or equivalent explicitly implemented mechanism.

If the OpenFGA write succeeds but persisting the completed idempotency response fails, the reservation SHALL remain in a recoverable state.

A recovery worker or reconciliation process SHALL determine whether the OpenFGA write was applied before allowing a retry.

The service SHALL NOT blindly repeat the OpenFGA write while the result of the previous attempt is unknown.

### TTL

The default idempotency record TTL SHALL be 24 hours.

The TTL SHALL be configurable through service configuration.

The service SHALL persist `expires_at`.

After expiration, a completed response SHALL NOT be replayed.

After expiration, the key MAY be reused according to the cleanup and retention policy.

Cleanup SHALL NOT remove an actively processing reservation.

### Idempotency Test Requirements

The implementation SHALL include table-driven tests for:

- identical retry after successful completion;
- same key with different payload;
- concurrent reservation attempts;
- retry after TTL expiration;
- replay returning the exact stored response;
- replay not creating an audit record;
- replay not calling OpenFGA;
- conflict not creating an audit record;
- only the reservation owner calling OpenFGA.

## Single-action Authorization API

`POST /v0/decisions/check` accepts one action and returns one complete decision. It does not mutate business data, create users or write FGA tuples. The unfinished `/api/v0/resources/decisions/check` route is removed.

### Trusted caller and strict request

The caller SHALL present a valid active service-client JWT with scope `bot`. Scope is a space-separated list. Token verification precedes user lookup. `X-Telegram-User-Id` SHALL be a positive decimal identifier from the trusted bot route, mapped to an internal UUID. The mapped UUID SHALL equal subject.id and SHALL be used consistently in checks and audit.

Required JSON fields: request_id (nonzero UUID), subject.type (`user`), subject.id (nonzero UUID), action.name, resource.type, resource.id (nonzero UUID even for create). Unknown fields, unknown attributes, nulls and wrong types SHALL be rejected with 400. There is no subject.relation, is_admin or top-level user_id.

Optional subject.attributes accepts only telegram_user_id, which must match the header. Optional context accepts service/client/ip/trace_id/time; ip and time use IP/RFC3339 syntax and are informational only. Optional objects may be omitted or empty. Explicit null is invalid.

Action attributes: field_mask (nonempty array of nonempty names) is mandatory only for update; role is required only for member:add/edit and must be viewer/editor/admin. Resource attributes: space_id is required for note/reminder:create and member actions; optional for existing note/reminder, where it never replaces the stored parent. Space requests have no resource attributes; type comes from trusted synchronization, not this request.

### Action mapping

| Resource/action | Checked object | Relation |
|---|---|---|
| space:create | none | registered-user rule; future owner is the subject |
| space:read | space:resource.id | can_view |
| space:update | space:resource.id | can_manage_space |
| space:delete | space:resource.id | can_delete; owner of shared only |
| note:create | space:resource.attributes.space_id | can_create_note |
| note:read/update/delete | note:resource.id | can_view/can_update/can_delete |
| reminder:create | space:resource.attributes.space_id | can_create_reminder |
| reminder:read/update/delete/stop | reminder:resource.id | can_view/can_update/can_delete/can_stop |
| member:add/edit/remove | space:resource.attributes.space_id | can_manage_members |

For member actions resource.id is target_user_id; owner targets are forbidden for all three actions. Administrators may manage all other members including admin roles and their own membership. All member actions on personal are denied. Unknown objects return deny without a separate existence lookup. The check does not ensure that a new UUID or target user exists in the business store.

The order is authentication, structural validation, trusted identity, FGA permission, protected member-owner check, then Casbin for update. A relational deny short-circuits field checks. Any forbidden field denies the entire operation. Mask order and duplicate names do not change semantics. Technical/unknown nonempty field names cause 200 deny, while malformed masks cause 400.

### Options and replay

include_reasons defaults to false. consistency accepts higher_consistency (default) and minimize_latency, applied to every FGA query including target-owner checks. minimize_latency can observe stale relationships. Every request is computed again and receives a new decision_id, including requests reusing request_id. Auth-service does not cache decisions. Only space:create skips FGA, including when FGA is unavailable after startup.

### Authorization response

Completed checks return HTTP 200 with request_id, a new UUID decision_id, string result (allow/deny), matching boolean allow, reasons (always an array) and meta. Meta contains trusted subject, requested action/resource as type:UUID, policy_version, authorization_model_id and nonnegative latency_ms. For create/member the checked FGA object may differ from meta.resource and is recorded separately in audit. space:create reports the pinned model ID without claiming to have queried it.

With include_reasons=false, reasons is []. With true, reasons contains stable code/message objects: allowed_via_fga_relation and allowed_fields for an allowed update, allowed_for_registered_user for space:create, denied_by_fga_relation, forbidden_fields or protected_space_owner for a denial. Messages SHALL NOT claim an unobserved role-inheritance path. A denial explains the final failing stage, not previous successful stages.

Policy version is derived from the actual embedded Casbin model and policy. Invalid policy prevents readiness; a runtime policy error is a dependency failure, not a denial. decision_id is correlation metadata, not a consumable permit or a guarantee that permissions remain unchanged before a write.

### Error response

Check errors use `{error_code,message,request_id?}` without result/allow. request_id is included only when parsed. Invalid requests/headers/masks/attributes use 400 invalid_request; missing or invalid JWT uses 401 unauthorized; missing client access/scope uses 403 forbidden; subject mismatch uses 403 subject_mismatch; unknown Telegram user uses 404 user_not_found, without registration. Dependency failures use 503 identity_store_unavailable/openfga_unavailable/policy_unavailable. Unexpected failures use safe 500 internal_error.

This new envelope SHALL NOT replace existing synchronization/other endpoint envelopes. New synchronization invariant failures use the existing resource-envelope with validation HTTP 400; existing resource conflicts preserve HTTP 409.

### Space synchronization and migration

New create-space synchronization SHALL require resource.attributes.space_type (personal/shared), owner.type=user and owner.id equal to the trusted mapped subject. Owner and one type marker are written in a single FGA Write. Type is immutable through normal updates. Protected personal deletion, personal membership and changes to a space owner's membership SHALL be rejected before writes, regardless of decision_id. Missing/conflicting trusted type never permits a protected write. Checks use the same pinned FGA model as synchronization.

Existing spaces require an explicit trusted export of space_id,owner_id,space_type before activation. Migration SHALL support dry run and repeated application, validate completeness/ownership, and never infer shared from existing memberships. A compatible transitional model introduces metadata before the strict model is activated. Rollback disables the endpoint but does not restore permissive personal-space rules. This is not an idempotency or PostgreSQL/FGA atomicity guarantee.

## Scope Validation

The service SHALL validate requested scopes against the scopes allowed for the authenticated service client.

If the `scope` request parameter is omitted, the service SHALL use all scopes allowed for the service client.

If the `scope` request parameter is provided, every requested scope SHALL be included in the service client's allowed scopes.

If a requested scope is not allowed, the service SHALL reject the login request with an `invalid_scope` error.

The service SHALL return the effective scopes in the successful login response.

## Audit Record Contract

For every completed OpenFGA authorization check, including both `allow` and `deny` decisions, the service SHALL create an audit record.

Every authorization audit record SHALL contain:

- Caller identity.
- Internal subject user ID.
- Action.
- Checked OpenFGA object.
- Checked OpenFGA relation.
- Decision.
- Timestamp.
- Request ID.

For `member:add`, `member:remove`, and `member:edit`, the audit record SHALL also contain `target_user_id`.

For `member:add` and `member:edit`, the audit record SHALL also contain the requested role.

`UpdateResource` idempotency replay and conflict records SHALL NOT be authorization decision records. They MAY be recorded as operational audit events, but a replay SHALL NOT create a new authorization audit record.

Authentication, header-validation, identifier-validation, identity-resolution, idempotency, and infrastructure failures SHALL be logged without tokens, passwords, or other secrets.

## Authorization Scenarios

- An authorized note update with text,title returns allow; adding timestamp denies the entire operation.
- An authorized reminder update with remind_at,title returns allow; changing created/last_edit is denied.
- A personal-space owner receives deny for space:delete but may delete a note inside it.
- A foreign direct note owner cannot access a note inside personal; direct ownership in shared still works without membership.
- Registered users may create a new space without a FGA check; no business row or tuple is created by check.
- Member operations never change the owner's membership or assign owner; personal never accepts members.
- Dependency failures return an error, not allow/deny; RabbitMQ publication failure does not replace a completed decision.

## Planned Admin Scenarios

The following scenarios are not supported by the current OpenFGA model.

They SHALL remain planned until admin identity, admin authentication, resources, actions, OpenFGA relations, request contracts, and audit fields are specified.

### Scenario: Admin blocks user [PLANNED]

- GIVEN an admin has a valid authentication token and user-management permission
- WHEN the admin sends POST /users/{id}/block
- AND OpenFGA grants permission for the action
- THEN the system returns 200 OK
- AND the user is blocked
- AND the system creates an audit record

### Scenario: Admin responds to user appeal [PLANNED]

- GIVEN an admin has a valid authentication token and support permission
- WHEN the admin sends POST /appeals/{id}/respond with response text
- AND OpenFGA grants permission for the action
- THEN the system returns 200 OK
- AND the response is saved and visible to the user
- AND the system creates an audit record
## Completed-decision Audit

Every allow/deny logs caller, trusted subject, requested resource/action, checked object/relation, result, server time, request_id, decision_id and both policy versions. Member events add target_user_id and requested role where applicable. space:create records no checked FGA object/relation. A completed event is logged locally and sent to RabbitMQ on a bounded best-effort basis; delivery failure is logged without changing the result. Client context cannot replace trusted caller/time. Tokens, JWT claims, passwords and secrets are never logged.

## API and rollout references

The exact required/optional field table and checked JSON examples are in [docs/decisions-api.md](../../docs/decisions-api.md). The migration tool, trusted-export format, activation gates, and rollback procedure are in [docs/space-types-migration.md](../../docs/space-types-migration.md). Runtime `openfga.model_id` pins a model explicitly; without it the model selected at startup stays fixed for that process. It cannot be combined with `apply_model_on_start`. Startup compares the pinned model with the strict embedded model and refuses to serve check with the old/transition model.

The existing synchronization implementation currently writes resource creation tuples. BZ-167 adds space type and privacy guards, not general update/delete/member tuple writers or the separate idempotency mechanism. Normative sync requirements outside those implemented paths remain future work.
