# B1 — Auth and QR identity service

## Goal

Implement permanent account identity, registration and login, JWT authentication, and server-signed immutable QR payloads.

## Own

- `backend/internal/auth/`
- `backend/internal/qr/`
- `backend/migrations/001_users.sql`
- Focused tests for these packages and migration behavior.

Do not edit server startup or shared route registration. Export a constructor or route-registration function for the coordinator to wire into `backend/cmd/server/main.go`.

## Requirements

- Implement `POST /v1/auth/register`, `POST /v1/auth/login`, and authenticated `GET /v1/account/me`.
- Derive `accountId` from the public key as described by the plan. Sign the exact QR identity payload and provide a verification function.
- Store password hashes, never plaintext passwords. Do not expose or regenerate private key material.
- Make `qrPayload` write-once at the database layer as well as in the API.
- Keep migration `001` limited to the base `users` table and auth-owned fields. B3 adds scan counters in its own migration.
- Provide JWT middleware or an auth helper that exposes the authenticated account ID. Document the JWT claims and helper signature for B2, B3, B4, and B5.

The plan permits a server-generated key pair or a client-generated public key. Use the frozen contract if it chooses one; if not, report the choice before locking the API.

## Dependencies and handoff

- No implementation dependency.
- B2 needs a stable JWT claim shape and authentication helper.
- B3, B4, and B5 need the account ID and base user columns.
- C1 needs the registration/login response shape and QR payload format.

## Acceptance

- Duplicate registration with the same public key fails.
- QR signature verifies and payload remains unchanged after account creation.
- Invalid credentials and invalid tokens fail explicitly.
- Auth package tests cover success and failure paths.

## Report

Use the report format in [`../README.md`](../README.md). Include the migration order, JWT claims, auth helper signature, endpoint request/response examples, and test command/results.
