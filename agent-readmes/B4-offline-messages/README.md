# B4 — Offline message queue

## Goal

Persist end-to-end encrypted chat messages for offline recipients and deliver them after reconnect.

## Own

- `backend/internal/messages/`
- `backend/migrations/004_offline_messages.sql`
- Offline-message tests.

Do not edit the signaling hub, shared route registration, B1 users migration, or transfer code.

## Requirements

- Create the `offline_messages` table with UUID message IDs, sender and recipient IDs, ciphertext bytes, timestamps, and delivery state.
- Implement authenticated `POST /v1/messages/offline` and `POST /v1/messages/ack`.
- Persist ciphertext only. Do not decrypt, inspect, or log message contents.
- Push undelivered messages on recipient reconnect through B2's published delivery hook. Do not edit B2 code.
- Keep transfer delivery out of this queue. Transfers reconcile through payment history.
- Define how acknowledgement marks delivery and how duplicate delivery is handled; align event shape with the shared WS protocol.

The plan's description of which side detects an offline recipient is ambiguous. Use the frozen API contract if available; otherwise report the ambiguity and keep storage/delivery services behind an interface.

## Dependencies and handoff

- B1 provides auth identity and user references.
- B2 provides the authenticated-connect delivery hook.
- C2 consumes offline-message events and sends acknowledgements.

## Acceptance

- A stored message reaches the recipient after WebSocket reconnect.
- Acknowledgement is idempotent.
- Unauthorized users cannot enqueue or acknowledge another user's message.
- Tests verify ciphertext round-trip without plaintext exposure.

## Report

Include endpoint shapes, service signatures, delivery-hook contract, migration details, and exact test results.
