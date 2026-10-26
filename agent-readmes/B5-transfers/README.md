# B5 — Transfers service

## Goal

Implement connected-peer shiny-rock transfers with atomic accounting, idempotent retries, history, and WebSocket notifications.

## Own

- `backend/internal/payments/`
- `backend/migrations/005_transfers.sql`
- Transfer-specific tests.

Do not edit B2 signaling, B3 ledger/connections, B1 users schema, or shared server startup.

## Requirements

- Implement authenticated `POST /v1/payments/send` and `GET /v1/payments/history`.
- Validate UUID v4 `transferId`, distinct sender and recipient, positive amount, maximum two decimal places, amount bounds, and note length if defined by the contract.
- Check the connection through B3's published interface. Reject unconnected and self-transfers.
- Use decimal arithmetic and PostgreSQL `NUMERIC(20, 8)`. Never use floating-point balances.
- Debit and credit atomically. Lock the sender row and never allow a negative balance.
- Make `transferId` idempotent, including concurrent retries. Reuse the original result for a retry by the original sender; reject reuse by another sender.
- Apply the planned per-sender rate limit.
- Notify B2's `SendTo` only after commit. Offline recipients reconcile through history, not B4's queue.
- History supports optional peer, `since`, and bounded `limit` filters.

The plan has a response-status inconsistency: this service specifies `200` for an existing idempotent transfer, while C6 mentions `409`. Keep the backend behavior at `200` unless the coordinator explicitly changes the shared contract.

## Dependencies and handoff

- B1 provides users and authenticated account identity.
- B2 provides `SendTo(accountID, payload)`.
- B3 provides the connection-check API and balance schema.
- C2 and C6 consume transfer/history response shapes.

## Acceptance

Test happy path, insufficient funds, self-transfer, unconnected recipient, duplicate and concurrent duplicate IDs, rate limiting, history filters, and zero-sum transfer accounting.

## Report

Include public interfaces, migrations, request/response examples, idempotency behavior, and exact test results.
