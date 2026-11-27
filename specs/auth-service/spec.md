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
3. Single-action authorization through `POST /api/v0/decisions/check`.

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
- Every completed decision SHALL require a successful OpenFGA call. Shared space creation reads the pinned model. Other operations check a mapped relation on the target object using the internal user ID. For personal space creation, check the verified user’s has_personal_space relation with higher consistency and allow creation only when the relation is absent.
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

For note/reminder, a foreign personal parent denies read/update/delete/stop even if another shared parent would grant access. The model applies this exclusion to every public record permission. OpenFGA does not enforce a single parent; the exclusion protects personal privacy even with inconsistent or concurrently added parent tuples. This does not implement parent uniqueness or UpdateResource idempotency.

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

An identical retry SHALL return the stored `UpdateResourceResponse` only when the reservation is completed. While the reservation is active, the retry SHALL return HTTP 409 `idempotency_in_progress` immediately. Each attempt SHALL emit its own HTTP request audit event.

An identical retry SHALL NOT create a new authorization decision or duplicate its decision audit record.

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

Each rejected conflict attempt SHALL produce exactly one terminal operational HTTP audit event. It SHALL NOT create a new authorization decision.

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
- replay producing one HTTP request audit event and no additional authorization decision;
- replay not calling OpenFGA;
- conflict returning HTTP 409 with exactly one terminal operational HTTP audit event for the rejected attempt, no new authorization decision and no OpenFGA writes;
- only the reservation owner calling OpenFGA.

## Single-action Authorization API

`POST /api/v0/decisions/check` accepts one action and returns one complete decision. It does not mutate business data, create users or write FGA tuples. The unfinished `/api/v0/resources/decisions/check` route is removed.

### Trusted caller and strict request

Decision checks, note filtering and resource updates SHALL require a valid active service-client JWT with scope `bot`. Scope is a space-separated list. Token verification precedes user lookup. `X-Telegram-User-Id` SHALL be a positive decimal identifier from the trusted bot route, mapped to an internal UUID. The mapped UUID SHALL equal subject.id and SHALL be used consistently in checks and audit.

Required JSON fields: request_id (nonzero UUID), subject.type (`user`), subject.id (nonzero UUID), action.name, resource.type, resource.id (nonzero UUID even for create). Unknown fields, unknown attributes, nulls and wrong types SHALL be rejected with 400. There is no subject.relation, is_admin or top-level user_id.

Optional subject.attributes accepts only telegram_user_id, which must match the header. A mismatch in this diagnostic attribute returns HTTP 400 invalid_request; subject.id differing from the trusted database identity returns HTTP 403 subject_mismatch. Optional context accepts service/client/ip/trace_id/time; ip and time use IP/RFC3339 syntax and are informational only. Optional objects may be omitted or empty. Explicit null is invalid.

Action attributes: field_mask (nonempty array of nonempty names) is mandatory only for update; role is required only for member:add/edit and must be viewer/editor/admin. Resource attributes: space_id is required for note/reminder:create and member actions; optional for existing note/reminder, where it never replaces the stored parent. space:create requires resource.attributes.space_type (shared or personal). Other actions reject this attribute. The type of an existing space comes from trusted synchronization.

### Action mapping

| Resource/action | Checked object | Relation |
|---|---|---|
| space:create | shared: read pinned model; personal: user:<subject>#has_personal_space | shared → allow; personal → allow only when absent; future owner is the verified subject |
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

include_reasons defaults to false. consistency accepts higher_consistency (default) and minimize_latency, applied to FGA queries including target-owner checks. A personal space creation check always uses higher_consistency. minimize_latency can observe stale relationships. Every request is computed again and receives a new decision_id, including requests reusing request_id. Auth-service does not cache decisions. For shared space creation the service reads the pinned model. When that read fails it returns 503 openfga_unavailable and does not save a decision.

### Authorization response

Before returning a completed check, auth-service SHALL insert its allow or deny into PostgreSQL auth.decisions, keyed by decision_id. Store request_id (nonunique), database creation time, authenticated caller, internal subject UUID, action and its attributes, requested resource, requested space_type for space:create, and parent ID if supplied, checked FGA object/relation if applicable, result, computed reasons and both policy versions. include_reasons governs only the response. An insert failure SHALL return HTTP 503 decision_store_unavailable without a decision. An invalid or unauthorized request does not produce a decision row. Operation-time verification is a separate change.

When authentication rejects a decision check, the middleware SHALL leave the body unread. The optional X-Request-ID request header supplies the correlation ID when it contains one nonzero UUID. The service SHALL return that ID in the JSON request_id on authentication rejection. With a missing, invalid, zero or repeated header, the service SHALL generate a UUID and omit request_id from that rejection JSON. The X-Request-ID response header, warning and terminal audit event SHALL use the same correlation UUID. After authentication the required JSON request_id identifies the decision, including when it differs from the header.

Completed checks return HTTP 200 with request_id, a new UUID decision_id, string result (allow/deny), matching boolean allow, reasons (always an array) and meta. Meta contains trusted subject, requested action/resource as type:UUID, policy_version, authorization_model_id and nonnegative latency_ms. For create/member the checked FGA object may differ from meta.resource and is recorded separately in audit. For shared space creation, meta.authorization_model_id identifies the pinned model read from OpenFGA. The service performs no object or relation check for this operation and leaves both audit values empty.

With include_reasons=false, reasons is []. With true, reasons contains stable code/message objects: allowed_via_fga_relation and allowed_fields for an allowed update, allowed_for_registered_user for shared space:create, first_personal_space or personal_space_already_exists for personal space:create, denied_by_fga_relation, forbidden_fields or protected_space_owner for a denial. Messages SHALL NOT claim an unobserved role-inheritance path. A denial explains the final failing stage, not previous successful stages.

With include_reasons=true, relational allow/deny messages SHALL identify the checked OpenFGA relation, object and trusted subject, including the parent space used for create/member. They SHALL describe the boolean check outcome without claiming a directly stored tuple, role-inheritance path or object existence. Allowed-fields messages SHALL list the requested fields that passed policy validation, sorted and deduplicated, together with resource type/action; they are not the complete policy allowlist. Forbidden-fields messages SHALL identify the resource type/action and the denied fields. Protected-owner messages SHALL identify the target user and checked space. Reason codes, decision semantics and reasons=[] when explanations are disabled remain unchanged.

Policy version is derived from the actual embedded Casbin model and policy. Invalid policy prevents readiness; a runtime policy error is a dependency failure, not a denial. decision_id is correlation metadata, not a consumable permit or a guarantee that permissions remain unchanged before a write.

### Error response

Check errors use `{error_code,message,request_id?}` without result/allow. request_id is included only when parsed. Invalid requests/headers/masks/attributes use 400 invalid_request; missing or invalid JWT uses 401 unauthorized; missing client access/scope uses 403 forbidden; subject mismatch uses 403 subject_mismatch; unknown Telegram user uses 404 user_not_found, without registration. Dependency failures use 503 identity_store_unavailable/openfga_unavailable/policy_unavailable/decision_store_unavailable. Unexpected failures use safe 500 internal_error.

Validation errors SHALL keep HTTP 400 and error_code="invalid_request" while identifying the invalid header or body field and the violated constraint in message. Missing or empty X-Telegram-User-Id SHALL report "X-Telegram-User-Id header is required"; malformed IDs SHALL describe the positive decimal int64 format. The optional subject.attributes.telegram_user_id SHALL be identified when malformed or different from the header. Known DTO, JSON decoding and operation-validation failures SHALL name their JSON path and expected type/constraint, including invalid/empty UUIDs, null and array elements. For example, an empty member target ID SHALL report "resource.id must be a nonzero UUID". Unknown fields SHALL identify the containing known object without reflecting the untrusted field name. Unreadable, oversized or syntactically malformed JSON bodies SHALL have a safe body-level explanation; syntax errors MAY include a byte offset. For POST /api/v0/decisions/check, every JSON object SHALL use the exact field names defined by its DTO. Each name may appear only once per object. A noncanonical spelling, repeated key or case collision SHALL return HTTP 400 invalid_request even when the values match. The service SHALL reject the request before decision evaluation, make no OpenFGA or field-policy call, and save no decision. Null remains invalid anywhere. On a rejected body, only a single canonical, successfully parsed nonzero request_id may identify the request. If that key is repeated or has a case variant, the error body SHALL omit request_id and the header and audit SHALL use transport correlation. Return one validation failure at a time. Public messages SHALL contain only deliberately constructed validation details, without supplied values, raw parser/dependency errors or internal wrapping context. The authentication order, error envelope and dependency-error messages remain unchanged.

This new envelope SHALL NOT replace existing synchronization/other endpoint envelopes. New synchronization invariant failures use the existing resource-envelope with validation HTTP 400; existing resource conflicts preserve HTTP 409.

### Space synchronization and migration

New create-space synchronization SHALL require resource.attributes.space_type (personal/shared), owner.type=user and owner.id equal to the trusted mapped subject. One transactional FGA Write stores the owner, the space type and the UUID marker `space:<id>#created@user:*`. Every create uses the same marker for that space ID across all owners and types. With duplicate writes rejected, competing creates for the same ID yield one successful synchronization and HTTP 409 for the other. Only the successful request contributes tuples. A decision check does not reserve the UUID. A future shared-space deletion writer must remove the UUID marker in the same transaction as the space relationships. For a personal space the same transaction inserts user:<owner>#has_personal_space@user:<owner>. A duplicate marker rejects the entire write. Type is immutable through normal updates. Protected personal deletion, personal membership and changes to a space owner's membership SHALL be rejected before writes, regardless of decision_id. Missing/conflicting trusted type never permits a protected write. Checks use the same pinned FGA model as synchronization.

Existing spaces require an explicit trusted export of space_id,owner_id,space_type before activation. Migration SHALL support dry run and repeated application, validate completeness and ownership, and add missing type, UUID and personal-owner markers from the verified records. Before reopening writes, all writers must enforce the new creation transaction and a verification read must confirm full backfill. A compatible transitional model introduces metadata before the strict model is activated. Rollback disables the endpoint but does not restore permissive personal-space rules. This is not an idempotency or PostgreSQL/FGA atomicity guarantee.

## Scope Validation

The service SHALL validate requested scopes against the scopes allowed for the authenticated service client.

If the `scope` request parameter is omitted, the service SHALL use all scopes allowed for the service client.

If the `scope` request parameter is provided, every requested scope SHALL be included in the service client's allowed scopes.

If a requested scope is not allowed, the service SHALL reject the login request with an `invalid_scope` error.

The service SHALL return the effective scopes in the successful login response.

## Audit Record Contract

For every completed OpenFGA authorization check, including both `allow` and `deny` decisions, the service SHALL create an audit record.

Every completed authorization decision audit record SHALL contain:

- Caller identity.
- Internal subject user ID.
- Action.
- Checked OpenFGA object and relation when a relation was evaluated. Shared space creation only checks model availability and leaves both values empty.
- Decision.
- Timestamp.
- Request ID.

For `member:add`, `member:remove`, and `member:edit`, the audit record SHALL also contain `target_user_id`. After the additional owner check completes it SHALL include the boolean `target_is_owner`. Omit this field if the check was skipped or failed.

For `member:add` and `member:edit`, the audit record SHALL also contain the requested role.

For the future `UpdateResource` idempotency mechanism, each replay or conflict attempt SHALL be recorded as one terminal operational HTTP audit event. Neither outcome creates a new authorization decision or writes to OpenFGA.

Authentication, header-validation, identifier-validation, identity-resolution, idempotency, and infrastructure failures SHALL be logged without tokens, passwords, or other secrets.

For `auth/notes/filter` and `resources/update`, the failed audit event SHALL retain the level and a safe textual `cause` of the first failure. Token rejection and note filtering failures use `warn`. Resource validation uses `debug`. A failed tuple write uses `error`. A later HTTP response failure SHALL preserve the first failure's level and cause.

## Authorization Scenarios

- An authorized note update with text,title returns allow; adding timestamp denies the entire operation.
- An authorized reminder update with remind_at,title returns allow; changing created/last_edit is denied.
- A personal-space owner receives deny for space:delete but may delete a note inside it.
- The `personal` and `shared` wildcard tuples describe the space type for all users. A public permission also requires the appropriate owner or membership relation. Missing or conflicting type markers deny space permissions.
- For a note or reminder, `accessible_owner` requires direct ownership of the record. In shared this works without membership. In personal the record owner must also own the parent space.
- Registered users may create shared spaces. A verified user may create a first personal space. An existing owner marker denies the request. The check itself is read-only.
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
## Decision attempt audit

For decision checks, resource synchronization and note filtering, audit starts as soon as the HTTP router selects the user operation, before request logging, metrics, authentication or decoding. A short circuit or panic before authentication SHALL produce a failed terminal event with stage `receive_request` even if the middleware already wrote an HTTP response and returned nil. Calling a user operation without its active audit event SHALL fail before JWT verification, identity lookup, evaluation or persistence, including direct service calls. Startup, dependency initialization and operator CLI commands use diagnostic logging. Each decision check attempt SHALL emit one terminal local domain audit event for operation `check_decision`, including authentication, body decoding, validation, identity lookup, OpenFGA, field policy, persistence and response-write failures. The event SHALL carry a nonzero correlation UUID, stage, status and safe error code/message on failure. A known request_id SHALL match the HTTP correlation ID. Only verified identities and validated operation metadata may be recorded. Failures before evaluation completes SHALL omit decision_id and decision. Once allow or deny is computed, the audit event SHALL retain decision_id, decision, policy_version and authorization_model_id. If PostgreSQL does not confirm the write, the event SHALL record status failed, stage save_decision and error_code decision_store_unavailable. The client SHALL receive HTTP 503 with the error and request_id. The audit identifier is available for reconciliation with PostgreSQL. If writing the response fails after persistence, keep the saved decision metadata. Preserve an earlier failure alongside any response-write failure. Panics SHALL be audited and propagated. Finalization uses a bounded context independent of request cancellation; RabbitMQ delivery is best effort after local logging.


Every allow/deny logs caller, trusted subject, requested resource/action, checked object/relation, result, server time, request_id, decision_id and both policy versions. Member events add target_user_id and requested role where applicable. Shared space creation records no checked FGA object/relation. Personal space creation records the verified user object and has_personal_space relation. A completed event is logged locally and sent to RabbitMQ on a bounded best-effort basis; delivery failure is logged without changing the result. Client context cannot replace trusted caller/time. Tokens, JWT claims, passwords and secrets are never logged.

## API and rollout references

The exact required/optional field table and checked JSON examples are in [docs/decisions-api.md](../../docs/decisions-api.md). The migration tool, trusted-export format, activation gates, and rollback procedure are in [docs/space-types-migration.md](../../docs/space-types-migration.md). Runtime `openfga.model_id` pins a model explicitly; without it the model selected at startup stays fixed for that process. It cannot be combined with `apply_model_on_start`. Startup compares the pinned model structurally with the strict embedded protobuf model and refuses to serve check with the old/transition model. Defaults added by OpenFGA are normalized, but reordered repeated nodes can still fail comparison; this is an exact model contract, not a proof of equivalent permissions. Individual relation checks, startup model verification and a paginated tuple read use openfga.request_timeout (default 5s), shortened by an earlier caller deadline. Decision audit delivery uses audit.delivery_timeout (default 1s). The CLI exposes request-timeout and timeout flags for the dependency and whole migration budgets. Timeout is a dependency error, never a completed deny.

The existing synchronization implementation currently writes resource creation tuples. BZ-167 adds space type and privacy guards, not general update/delete/member tuple writers or the separate idempotency mechanism. Normative sync requirements outside those implemented paths remain future work.

### BZ-167 review: boundaries and testing

DTO validation covers only input structure, required base fields and formats. The decision service validates resource/action combinations, conditional field_mask/space_id/space_type/role and allowed membership roles before identity/FGA calls; HTTP error semantics remain unchanged. The check route is registered in the existing api/v0 group as /api/v0/decisions/check; /v0/decisions/check is not an alias. The service runs fast Go unit tests separately from component integration tests of its CLI, migrations, policies, adapters and PostgreSQL storage. Pytests verifies public HTTP and messaging contracts, business scenarios and dependency failures against the standard service image. Testkit manages the isolated environment.
