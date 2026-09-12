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

- OpenFGA is the authoritative relationship and authorization model for spaces, notes, reminders, and members.
- The current implemented resource synchronization API writes relationships to OpenFGA.
- The request-based authorization check API is planned and is not currently implemented.

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
- A future authorization check API SHALL query OpenFGA using the internal user ID, the mapped relation, and the target object.
- A future authorization check API SHALL return an `allow` or `deny` decision.
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

### Action Categories (Planned)

The following actions belong to the planned authorization check API.

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

### Deletion Semantics (Planned)

The following rules apply to the planned deletion checks.

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

Casbin MAY be used for future domain-specific authorization rules that are not expressible in OpenFGA.

Casbin SHALL NOT replace or duplicate OpenFGA authorization for spaces, notes, reminders, or members.

The existing `internal/service/politics` implementation is legacy and is not an active authorization source.

The `internal/service/enforcer` package is reserved for future Casbin integration and SHALL NOT be used as an alternative implementation of the OpenFGA model.

The root `casbin_model.conf` is reserved for planned Casbin-based domain-specific rules.

It SHALL NOT be used for current space, note, reminder, or member authorization.

It SHALL NOT be treated as the source of truth while OpenFGA is authoritative for those resources.

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

## UpdateResource Idempotency

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

## Authorization Check Mapping (Planned)

The following mapping describes the intended future authorization check API.

It is not part of the current supported API.

For each authorization request, the future service SHALL:

1. Validate the action and its required identifiers.
2. Validate the bot token.
3. Resolve `X-Telegram-User-Id` to an internal user ID.
4. Map the action to the OpenFGA object and relation below.
5. Perform the OpenFGA check.
6. Use the OpenFGA result as the authorization decision.
7. Create an audit record for a completed allow or deny check.

| Action | Required request fields | OpenFGA object | Required relation |
|---|---|---|---|
| `note:create` | `space_id` | `space:<space_id>` | `can_create_note` |
| `note:read` | `note_id` | `note:<note_id>` | `can_view` |
| `note:update` | `note_id` | `note:<note_id>` | `can_update` |
| `note:delete` | `note_id` | `note:<note_id>` | `can_delete` |
| `reminder:create` | `space_id` | `space:<space_id>` | `can_create_reminder` |
| `reminder:read` | `reminder_id` | `reminder:<reminder_id>` | `can_view` |
| `reminder:update` | `reminder_id` | `reminder:<reminder_id>` | `can_update` |
| `reminder:stop` | `reminder_id` | `reminder:<reminder_id>` | `can_stop` |
| `reminder:delete` | `reminder_id` | `reminder:<reminder_id>` | `can_delete` |
| `member:add` | `space_id`, `target_user_id`, `role` | `space:<space_id>` | `can_manage_members` |
| `member:remove` | `space_id`, `target_user_id` | `space:<space_id>` | `can_manage_members` |
| `member:edit` | `space_id`, `target_user_id`, `role` | `space:<space_id>` | `can_manage_members` |

For note and reminder read, update, delete, and stop actions, `space_id` MAY be included as optional audit context but SHALL NOT replace `note_id` or `reminder_id` in the OpenFGA check.

## Authorization Response Contract (Planned)

This contract applies only to completed authorization checks.

A successful authorization response SHALL contain:

| Field | Required | Rules |
|---|---:|---|
| `decision` | Yes | `allow` or `deny` |
| `reason_code` | For `deny` only | Machine-readable denial code |
| `reason_code` | For `allow` | Optional |

REQ-RESPONSE-001: The system SHALL return a `decision` field with value `allow` or `deny` for every completed authorization check.

REQ-RESPONSE-002: For `deny` decisions, the system SHALL include a `reason_code` field.

REQ-RESPONSE-003: For `allow` decisions, the system MAY include a `reason_code` field.

## Error Response Contract

The Error Response Contract applies to authentication, validation, identity-resolution, idempotency, dependency, and infrastructure failures.

Error responses SHALL NOT be treated as authorization decisions and SHALL NOT be required to contain `decision` or `reason_code`.

The error response SHALL contain:

| Field | Required | Rules |
|---|---:|---|
| `error_code` | Yes | Stable machine-readable code |
| `message` | Yes | Human-readable error message |
| `request_id` | When available | Request correlation identifier |

For OpenFGA unavailability:

- HTTP status SHALL be 503 Service Unavailable.
- `error_code` SHALL be `openfga_unavailable`.
- The response SHALL NOT contain `decision`.
- The response SHALL NOT be required to contain `reason_code`.
- The failure SHALL be logged without secrets.

For an idempotency conflict:

- HTTP status SHALL be 409 Conflict.
- `error_code` SHALL be `idempotency_conflict`.
- The response SHALL NOT contain `decision`.
- The response SHALL NOT be required to contain `reason_code`.

For an active idempotency reservation:

- HTTP status SHALL be 409 Conflict.
- `error_code` SHALL be `idempotency_in_progress`.
- The response SHALL NOT contain `decision`.
- The response SHALL NOT be required to contain `reason_code`.

## Scope Validation

The service SHALL validate requested scopes against the scopes allowed for the authenticated service client.

If the `scope` request parameter is omitted, the service SHALL use all scopes allowed for the service client.

If the `scope` request parameter is provided, every requested scope SHALL be included in the service client's allowed scopes.

If a requested scope is not allowed, the service SHALL reject the login request with an `invalid_scope` error.

The service SHALL return the effective scopes in the successful login response.

## Audit Record Contract

For every completed planned OpenFGA authorization check, including both `allow` and `deny` decisions, the service SHALL create an audit record.

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

## Planned Authorization Scenarios

The following scenarios describe the planned authorization check API and are not current implemented API scenarios.

### Scenario: Delete note - allowed for note owner

- GIVEN a bot with a valid authentication token
- AND the request includes `X-Telegram-User-Id: 123456`
- AND Telegram user ID 123456 is mapped to internal user ID `550e8400-e29b-41d4-a716-446655440000`
- AND internal user `550e8400-e29b-41d4-a716-446655440000` is the owner of `note:<note-uuid>`
- WHEN the bot sends an authorization request with action="note:delete" and note_id=<note-uuid>
- AND OpenFGA confirms relation `can_delete` for the internal user on `note:<note-uuid>`
- THEN the system returns 200 OK with decision="allow"
- AND the response MAY contain reason_code="allowed"
- AND the system writes an audit record with the checked object `note:<note-uuid>` and relation `can_delete`

### Scenario: Delete note - denied for editor who is not owner

- GIVEN a bot with a valid authentication token
- AND the request includes `X-Telegram-User-Id: 456789`
- AND Telegram user ID 456789 is mapped to internal user ID `6ba7b810-9dad-11d1-80b4-00c04fd430c8`
- AND the internal user is an editor in the containing space
- AND the internal user is not the owner of `note:<note-uuid>`
- WHEN the bot sends an authorization request with action="note:delete" and note_id=<note-uuid>
- AND OpenFGA denies relation `can_delete` for the internal user on `note:<note-uuid>`
- THEN the system returns 200 OK with decision="deny"
- AND the response includes reason_code="insufficient_permissions"
- AND the system writes an audit record with the checked object `note:<note-uuid>` and relation `can_delete`

### Scenario: Add member - allowed

- GIVEN a bot with a valid authentication token
- AND the request includes `X-Telegram-User-Id: 123456`
- AND Telegram user ID 123456 is mapped to internal user ID `550e8400-e29b-41d4-a716-446655440000`
- AND the internal user can manage members in `space:<space-uuid>`
- WHEN the bot sends an authorization request with action="member:add", space_id=<space-uuid>, target_user_id=<target-user-uuid>, and role="editor"
- AND OpenFGA confirms relation `can_manage_members` on `space:<space-uuid>`
- THEN the system returns 200 OK with decision="allow"
- AND the system writes an audit record containing `target_user_id=<target-user-uuid>` and requested role="editor"

### Scenario: Authorization check dependency unavailable

- GIVEN a valid planned authorization request
- WHEN OpenFGA is unavailable or times out
- THEN the system returns 503 Service Unavailable
- AND the response contains `error_code="openfga_unavailable"`
- AND the response does not contain an authorization `decision`
- AND the system logs the dependency failure without logging secrets

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