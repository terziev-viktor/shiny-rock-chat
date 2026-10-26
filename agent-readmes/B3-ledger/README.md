# B3 — Scan ledger and connections

## Goal

Implement atomic first-scan rewards, balance reads, and canonical user connections.

## Own

- `backend/internal/ledger/`
- `backend/migrations/002_scan_events.sql`
- `backend/migrations/003_connections.sql`
- `backend/migrations/003b_user_scan_counters.sql`
- Ledger-specific tests.

Do not edit B1's `001_users.sql`, B2 signaling code, B5 payment code, or shared server startup.

## Requirements

- Implement `POST /v1/qr/scan` and authenticated `GET /v1/ledger/balance`.
- Validate signed QR payloads through B1's QR verification API and reject self-scans.
- A first scan of a directed `(scanner, scanned)` pair credits exactly `+1` to each user in one transaction. A repeat credits neither.
- Enforce uniqueness in PostgreSQL, not only application code. Handle concurrent duplicate requests without double credit.
- Generate a unique, monotonic `scanIndex` for each scanned user under concurrent scans.
- Create canonical `(user_a, user_b)` connection rows, with `user_a < user_b`.
- Expose a connection-check interface for B5. Keep connection reads independent of payment internals.
- Preserve the plan's per-user and global mint invariants. Transfers are zero-sum and do not change scan counters.

## Dependencies and handoff

- B1 owns the base `users` table, including `account_id` and `shiny_rock`.
- B5 needs the exact connection-check signature and table shape.
- C3 needs scan request/response shape.
- C4 needs balance response shape.

## Acceptance

Test first scan, repeat scan, reverse-direction scan, concurrent duplicate scan, concurrent distinct scanners, unique scan indices, and both accounting invariants from the plan. Use the repository's integration-test pattern; report clearly if PostgreSQL is unavailable.

## Report

Include migration order, public service/handler signatures, scan response examples, invariant queries, and exact test results.
