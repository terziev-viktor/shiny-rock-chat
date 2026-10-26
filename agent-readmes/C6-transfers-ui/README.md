# C6 — Transfers UI

## Goal

Let users send shiny-rock to connected peers from chat, with validation, optimistic feedback, idempotent retry, and history reconciliation.

## Own

- `client/lib/payments/`
- `client/lib/ui/chat/send_payment_button.dart`
- `client/lib/ui/chat/payment_send_dialog.dart`
- Payment repository and widget tests.

Do not edit `ChatScreen`, C2's chat-store implementation, C4 wallet UI, or `pubspec.yaml` without coordinator approval. Provide a standalone send widget and integration instructions for the coordinator.

## Requirements

- Use B5's `POST /v1/payments/send` and `GET /v1/payments/history` contracts.
- Validate positive amount, current balance, and at most two decimal places without converting through `double`. Limit notes to 140 characters.
- Generate a UUID v4 transfer ID before the request. Use the same ID for optimistic message, persistence, and every retry.
- Add a pending sent-payment message through C2's public chat-store API.
- On success, confirm/upsert the message and update wallet balance from `senderBalance`.
- On insufficient funds or authorization failure, remove the optimistic message and show a clear error.
- B5's planned idempotent retry returns `200` with the original transfer. Handle that path; do not depend on `409` unless the coordinator changes the shared contract.
- Keep uncertain network failures pending and retry with backoff using the same transfer ID. Avoid unbounded retry loops.
- Persist/upsert payment history by transfer ID and reconcile when the chat opens.

## Dependencies and handoff

- B5 provides request/response and error behavior.
- C2 provides message models and store methods.
- C4 provides the ChatScreen integration point and wallet refresh method.

## Acceptance

- A valid amount posts the same ID shown in the optimistic message and confirms it on success.
- Insufficient balance removes the pending message and displays an error.
- Retry attempts reuse the original ID.
- History merge is idempotent.
- Tests cover validation, success, failure, retry, and duplicate history.

## Report

Include public repository/widget APIs, integration instructions for `ChatScreen`, retry policy, and exact test results.
