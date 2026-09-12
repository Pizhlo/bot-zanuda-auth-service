# Project Constitution

## Immutable Constraints

The system SHALL use Go 1.26+ for all backend code.

The system SHALL store secrets in Vault only.

The system MUST NOT commit `.env` files, secret values, or credentials to version control.

The system SHALL require minimum 80% test coverage.

## Technology Boundaries

The system SHALL use PostgreSQL for persistent storage.

The system SHALL use OpenFGA for authorization decisions.

The system SHALL NOT use MongoDB or other databases.

The system MAY use Casbin for fine-grained authorization in specific domains.

The system MAY use Redis for caching.

The system MAY use RabbitMQ for message queue processing.

## Security Requirements

The system SHALL NOT log tokens, passwords, private keys, or other secrets.

The authorization service SHALL validate the bot token before processing `X-Telegram-User-Id`.

The authorization service SHALL map the Telegram user ID from `X-Telegram-User-Id` to an internal user ID before OpenFGA checks.

The authorization service SHALL use the resolved internal user ID for OpenFGA checks and authorization audit records.

The authorization service SHALL process `X-Telegram-User-Id` only when it is received through the trusted bot → web-server → authorization-service request chain.

The authorization service SHALL reject requests with an invalid or expired bot token before querying the application database or OpenFGA.

The authorization service SHALL reject malformed or missing `X-Telegram-User-Id` before querying OpenFGA.

`decision_id` SHALL NOT be treated as an authorization proof unless decision verification is explicitly implemented.

`decision_id` SHALL be a well-formed UUID referencing authorization metadata from an approved trusted component.

Until decision verification is implemented, the service SHALL validate the presence and format of `decision_id` but SHALL NOT use it as proof of authorization.

## Architecture Principles

The system SHALL log every allow or deny authorization decision for audit.

The system SHALL log authentication, validation, identity-resolution, idempotency, and infrastructure failures without logging secrets.

The system SHALL return authorization decisions as `allow` or `deny` and SHALL NOT delegate authorization decisions to clients.

For `deny` decisions, the system SHALL include a machine-readable `reason_code` field.

For `allow` decisions, the system MAY include a `reason_code` field.

The planned authorization check API SHALL use the action-to-object-and-relation mapping defined in `specs/auth-service/spec.md`.

For planned `member:*` actions, `target_user_id` SHALL be required.

For planned `member:add` and `member:edit` actions, `role` SHALL be required.

For planned member authorization audit records, the system SHALL record the checked OpenFGA object and relation, `target_user_id`, and the requested role when applicable.

The system SHALL query OpenFGA for every planned authorization request.

The system SHALL NOT cache authorization decisions unless an explicit cache and invalidation design is approved.

The system SHALL use the exact OpenFGA object type and relation mapped to each action.

The system SHALL NOT authorize note or reminder operations on an existing object using only `space_id`.

The system SHALL use OpenFGA for space, note, reminder, and member authorization.

The system MAY use Casbin for domain-specific authorization rules that are not expressible in OpenFGA.

Casbin usage SHALL be explicitly documented and SHALL NOT duplicate OpenFGA authorization logic.

The legacy `politics` service SHALL NOT be used as an authorization source.

The `internal/service/politics` package is legacy and SHALL NOT be used by new code.

The `internal/service/enforcer` package is reserved for future Casbin integration and SHALL NOT replace OpenFGA.

The root `casbin_model.conf` is reserved for future domain-specific Casbin rules and SHALL NOT be used for current space, note, reminder, or member authorization.

## Resource Synchronization Idempotency

Every `UpdateResourceRequest` SHALL be processed under an idempotency scope owned by the authenticated service client.

The idempotency scope SHALL contain the service-client identity and `idempotency_key`.

The service SHALL atomically reserve an idempotency key before writing tuples to OpenFGA.

The service SHALL calculate and persist a canonical fingerprint of the complete request payload, excluding transport-only metadata that is not part of the operation semantics.

The service SHALL persist the complete successful `UpdateResourceResponse` before making the result available for replay.

An identical retry SHALL return the persisted response and SHALL NOT create a new audit record or execute another OpenFGA write.

A retry with the same idempotency key and a different fingerprint SHALL return an idempotency conflict error and SHALL NOT execute an OpenFGA write.

Only the request that successfully reserves an idempotency key may execute the corresponding OpenFGA write.

The service SHALL define and enforce an idempotency record TTL.

Expired idempotency records SHALL NOT be replayed and SHALL become available for a new request after expiration according to the configured retention policy.

The idempotency store and the OpenFGA write operation SHALL provide an explicit failure and recovery policy. The system SHALL NOT claim atomicity between PostgreSQL and OpenFGA unless a transactional-outbox or equivalent recovery mechanism is implemented.

If the OpenFGA write succeeds but persisting the completed idempotency response fails, the reservation SHALL remain in a recoverable state.

A recovery worker or reconciliation process SHALL determine whether the OpenFGA write was applied before allowing a retry.

The service SHALL NOT blindly repeat the OpenFGA write while the result of the previous attempt is unknown.

## Error Contracts

Successful authorization decisions and authorization denials SHALL use the Authorization Response Contract in `specs/auth-service/spec.md`.

Infrastructure failures, including OpenFGA unavailability, SHALL use the Error Response Contract and SHALL NOT be required to contain `decision` or `reason_code`.

Idempotency conflicts SHALL use the Error Response Contract and SHALL return HTTP 409 Conflict.

## Code Quality

All handlers SHALL have table-driven unit tests.

All public functions SHALL have documentation comments.

All externally visible request contracts SHALL be validated before external side effects are performed.

Idempotency logic SHALL have table-driven tests for replay, payload conflict, concurrent reservation, and TTL expiration.

## CI/CD Requirements

All commits SHALL pass `make lint` validation.

All pull requests SHALL have passing tests.

All changes to the OpenFGA model SHALL update `specs/auth-service/spec.md` and tests.

All changes to authorization action mappings SHALL update `specs/auth-service/spec.md`, handlers, and tests.

CI checks SHALL validate the canonical specification at `specs/auth-service/spec.md`.