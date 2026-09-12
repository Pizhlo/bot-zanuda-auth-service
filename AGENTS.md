# Agent Context

## Canonical Specification

The canonical authorization-service specification is:

```text
specs/auth-service/spec.md
```

Do not use a bare `spec.md` reference in code, tests, CI rules, or documentation.

## Tech Stack

- Go 1.26+
- PostgreSQL 16
- OpenFGA
- Casbin (for domain-specific authorization rules)
- RabbitMQ
- Vault
- Redis may be used for caching in the future
- Docker and Docker Compose
- Make

## Key Commands

```bash
make build            # compile the service
make test             # run unit tests with coverage
make lint             # run golangci-lint
make containers-up    # start the local environment
make containers-down  # stop the local environment
```

## Project Structure

- `cmd/` — application entry points.
- `internal/` — private application code, handlers, services, models, repositories, integrations, and idempotency storage.
- `pkg/` — public libraries, if any.
- `specs/auth-service/spec.md` — canonical authorization-service specification.
- `specs/` — other project specifications.
- `fga/` — OpenFGA model files.
- `fga/auth-model.fga` — canonical OpenFGA authorization model.

## Architecture Context

- This is a centralized authorization service.
- The bot is the currently supported service client.
- Future service clients may include an admin panel.
- Tokens are issued to service clients; Telegram end users do not receive service tokens.
- Secrets are stored in Vault only.
- The bot manages notes and reminders in personal and shared spaces.
- The current implemented APIs are service-token issuance and resource/relationship synchronization to OpenFGA.
- OpenFGA is the authoritative relationship and authorization model for spaces, notes, reminders, and members.
- The current implemented resource synchronization API writes relationships to OpenFGA.
- Casbin MAY be used for domain-specific authorization rules that are not expressible in OpenFGA.
- The request-based authorization check API is planned and SHALL NOT be treated as implemented.
- The canonical OpenFGA model is `./fga/auth-model.fga`.

## Implemented Service Responsibilities

- Issue JWT service tokens through the `client_credentials` flow.
- Validate active service clients and requested scopes.
- Obtain client secrets from Vault.
- Synchronize spaces, notes, reminders, owners, parent relations, and membership relations to OpenFGA.
- Enforce idempotency for `UpdateResource`.
- Preserve idempotency scope, canonical request fingerprint, processing ownership, stored response, and expiration time.
- Return applied OpenFGA tuple changes and the authorization model ID.

## Required Headers

- `Authorization` — service-client Bearer token.
- `X-Telegram-User-Id` — numeric Telegram user ID when a supported bot request is made on behalf of a Telegram user.

## Trusted Request Chain

- The bot receives Telegram updates from Telegram.
- The bot derives Telegram user identity only from a verified Telegram update.
- The bot sends its bot token and `X-Telegram-User-Id` to the web server.
- The web server is a transport proxy for bot authorization requests and has no separate authorization-service identity in the current design.
- The web server forwards the bot token and `X-Telegram-User-Id` only for requests received through the internal bot-to-web-server route.
- The web server MUST NOT forward `X-Telegram-User-Id` supplied by an external HTTP client.
- The authorization service validates the forwarded bot token before processing `X-Telegram-User-Id`.
- After successful bot-token validation, the authorization service maps Telegram user ID to an internal user ID in PostgreSQL.
- OpenFGA checks and planned authorization audit records use the internal user ID, not the Telegram user ID.

## Resource Synchronization Contract

- `UpdateResourceRequest` is the contract for OpenFGA relationship synchronization.
- The request includes `request_id`, `idempotency_key`, `decision_id`, `resource`, `relations`, `operation`, `change_type`, and `context`.
- `TelegramID` is transport metadata and is not serialized into JSON.
- `request_id` MUST be a valid UUID.
- `decision_id` MUST be a valid UUID and references authorization metadata from an approved trusted component.
- Until decision verification is implemented, `decision_id` is not an authorization proof.
- `idempotency_key` MUST be non-empty.
- `resource_added` must use `create`.
- `resource_removed` must use `delete`.
- `resource_moved` and `membership_changed` must use `update`.
- `membership_added` must use `create`.
- `membership_removed` must use `delete`.
- Notes and reminders require a parent `space` relation.
- Notes and reminders require an owner `user` relation when created.
- Do not change OpenFGA relations without preserving `UpdateResource` idempotency.

## UpdateResource Idempotency

- Scope idempotency by authenticated service-client identity and `idempotency_key`.
- Atomically reserve the key before calling OpenFGA.
- Calculate a canonical fingerprint of the complete semantic request payload.
- Persist the fingerprint, request ID, decision ID, processing state, creation time, expiration time, and complete response.
- Only the reservation owner may execute the OpenFGA write.
- An identical retry of a completed request returns the stored response.
- An identical replay must not create an audit record.
- An identical replay must not call OpenFGA.
- A different payload with the same scoped key returns HTTP 409 with `error_code="idempotency_conflict"`.
- A conflict must not call OpenFGA or create an authorization audit record.
- The default idempotency TTL is 24 hours unless configuration specifies another value.
- Do not claim PostgreSQL/OpenFGA atomicity unless an outbox, reconciliation, or equivalent recovery mechanism is implemented.
- Add table-driven tests for successful replay, payload conflict, concurrent reservation, TTL expiration, exact stored-response replay, no duplicate OpenFGA write, and no duplicate audit record.
- If OpenFGA succeeds but persisting the completed idempotency response fails, keep the reservation recoverable.
- A recovery worker or reconciliation process must determine whether the OpenFGA write was applied.
- Do not blindly repeat an OpenFGA write while the result of the previous attempt is unknown.

## Authorization Mapping (Planned)

The following mapping describes the planned authorization check API.

- `note:create` checks `can_create_note` on `space:<space_id>`.
- `note:read` checks `can_view` on `note:<note_id>`.
- `note:update` checks `can_update` on `note:<note_id>`.
- `note:delete` checks `can_delete` on `note:<note_id>`.
- `reminder:create` checks `can_create_reminder` on `space:<space_id>`.
- `reminder:read` checks `can_view` on `reminder:<reminder_id>`.
- `reminder:update` checks `can_update` on `reminder:<reminder_id>`.
- `reminder:stop` checks `can_stop` on `reminder:<reminder_id>` and is non-destructive.
- `reminder:delete` checks `can_delete` on `reminder:<reminder_id>` and is destructive.
- `member:add`, `member:remove`, and `member:edit` check `can_manage_members` on `space:<space_id>`.
- All `member:*` actions require `target_user_id`.
- `member:add` and `member:edit` also require `role`.
- Supported member roles are `viewer`, `editor`, and `admin`.
- `member:add` and `member:edit` MUST NOT assign the `owner` relation.
- Do not use `space_id` instead of `note_id` or `reminder_id` for checks on existing notes or reminders.

## Authorization Response Contract (Planned)

- A completed authorization check returns `decision="allow"` or `decision="deny"`.
- `reason_code` is mandatory for `deny`.
- `reason_code` is optional for `allow`.
- This contract does not apply to 4xx/5xx errors or dependency failures.

## Error Contract

- Errors use a stable machine-readable `error_code`.
- Errors include a human-readable `message`.
- Errors include `request_id` when available.
- OpenFGA unavailability returns HTTP 503 and `error_code="openfga_unavailable"`.
- Idempotency payload conflicts return HTTP 409 and `error_code="idempotency_conflict"`.
- An active idempotency reservation returns HTTP 409 with `error_code="idempotency_in_progress"`.
- Error responses do not require `decision` or `reason_code`.
- Do not return `allow` on OpenFGA errors.

## OpenFGA Integration

- Model file: `./fga/auth-model.fga`.
- Use the configured OpenFGA SDK and client.
- Planned authorization checks must query OpenFGA for every request.
- Do not cache authorization decisions unless a cache/invalidation design is explicitly approved.
- Treat OpenFGA unavailability as a failed authorization operation.
- Do not modify `fga/auth-model.fga` without updating `specs/auth-service/spec.md` and tests.
- All CI and validation rules must refer to `specs/auth-service/spec.md`, never to a bare `spec.md`.

## Audit Requirements

For every completed planned OpenFGA check, log:

- Caller identity.
- Internal subject user ID.
- Action.
- Checked OpenFGA object.
- Checked OpenFGA relation.
- Allow/deny decision.
- Timestamp.
- Request ID.

For member actions, also log `target_user_id`.

For member add/edit actions, also log requested `role`.

An idempotency replay is not a new authorization decision and must not create a duplicate authorization audit record.

Never log:

- Bearer tokens.
- JWT contents.
- Passwords.
- Private keys.
- Vault secret values.
- Database credentials.
- Other secrets.

## Testing Requirements

- Write table-driven tests for all handlers.
- Write table-driven tests for `UpdateResource` idempotency.
- Test identical replay.
- Test same key with a different payload.
- Test concurrent reservation attempts.
- Test TTL expiration.
- Test that only the reservation owner calls OpenFGA.
- Test that replay returns the exact stored response.
- Test that replay does not call OpenFGA.
- Test that replay does not create a duplicate audit record.
- Test that conflicts do not call OpenFGA.
- Use type-safe request and response models.
- Validate all external request fields before external side effects.
- Maintain at least 80% test coverage.

## Boundaries

**MUST:**

- Run `make lint` before committing.
- Run relevant tests after changes.
- Update `specs/auth-service/spec.md` and tests when changing the OpenFGA model, action mappings, request contracts, audit fields, idempotency behavior, or error contracts.
- Keep authorization checks fail-closed.
- Keep idempotency conflict handling fail-closed.
- Use the project logger.
- Files under `vault/` that contain certificates or keys MUST be excluded from commits unless they are explicitly non-secret test fixtures.
- Vault credentials and private keys MUST come from the runtime secret-management environment.

**SHALL NOT:**

- Commit `.env` files, secrets, tokens, or credentials.
- Use `fmt.Println` in production code.
- Skip tests in CI.
- Modify the OpenFGA model without updating `specs/auth-service/spec.md` and tests.
- Authorize an existing note or reminder based only on access to its space.
- Treat `reminder:stop` as reminder deletion.
- Allow a member-management request to assign the owner role.
- Use a bare `spec.md` path where `specs/auth-service/spec.md` is required.

## Legacy Components

- `internal/service/politics/` is legacy code.
- It is not an active authorization source.
- New code SHALL NOT depend on `internal/service/politics`.
- New authorization behavior SHALL use OpenFGA for spaces, notes, reminders, and members.
- The package MAY be removed in a separate cleanup change after all references are eliminated.

- `internal/service/enforcer/` is reserved for future Casbin integration.
- It SHALL NOT replace OpenFGA for spaces, notes, reminders, or members.
- The root `casbin_model.conf` is reserved for future domain-specific Casbin rules.

New authorization code SHALL NOT use `internal/service/politics`.

Do not extend or restore `politics` unless a separate migration specification explicitly requires it.

## Specification-First Rule

Before implementing any new feature, endpoint, authorization rule, idempotency behavior, error code, audit field, or OpenFGA relation change, the agent SHALL:

1. Check whether the change is explicitly described in `specs/auth-service/spec.md`.
2. If the change is not described, the agent SHALL propose adding it to the specification before implementation.
3. The agent SHALL NOT implement specification changes without first updating `specs/auth-service/spec.md` and, if applicable, `constitution.md`.
4. The agent SHALL treat the specification as the source of truth and SHALL NOT rely on implicit assumptions or undocumented behavior.

This rule applies to:

- new API endpoints or request/response fields;
- new authorization actions, objects, or relations;
- new error codes or HTTP status codes;
- new audit record fields;
- new idempotency rules or TTL values;
- new service scopes;
- changes to OpenFGA model semantics;
- changes to legacy component boundaries (`politics`, `enforcer`, Casbin).

The only exceptions are trivial refactors that do not change external behavior, error messages, or authorization logic.