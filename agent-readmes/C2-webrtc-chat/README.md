# C2 — WebRTC chat and signaling client

## Goal

Connect peers over WebRTC using B2 signaling, handle chat transport, and synchronize offline and payment events into the local chat store.

## Own

- `client/lib/webrtc/`
- `client/lib/chat/chat_service.dart`
- `client/lib/chat/payment_sync.dart`
- Chat message models and the public chat-store API.

Do not edit chat or wallet widgets, payment UI/repository, scanner UI, or native platform files.

## Requirements

- Connect to `/v1/ws?token=<JWT>` and implement the shared signaling message shapes.
- Establish a peer data channel and send/receive chat JSON.
- Handle offline-message events and acknowledgements.
- Upsert payment-sent/payment-received events by transfer ID. Reconnect duplicates must not create duplicate chat messages.
- Reconcile transfers from payment history on WS reconnect, using the last-seen timestamp.
- Expose a stable message stream and store methods for C4 and C6. Publish model fields and method signatures early.
- Use the dependency already present in `client/pubspec.yaml` unless it cannot meet the contract. Coordinate before changing dependency manifests.

## Dependencies and handoff

- B2 provides WS authentication, relay, and event shapes.
- B4 provides offline-message delivery behavior.
- B5 provides payment history.
- C3 calls the connect/open-peer API.
- C4 renders chat-store messages; C6 inserts optimistic payment messages.

## Acceptance

- Signals establish a data channel between test peers or a deterministic mock.
- Chat events flow through the public store.
- Duplicate payment events and reconnect reconciliation produce one message per transfer ID.
- Tests use injectable WS/WebRTC boundaries where platform hardware is unavailable.

## Report

Include the chat-store interface, connection lifecycle API, event mapping, and exact test results.
