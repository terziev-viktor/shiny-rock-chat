# B2 — WebSocket signaling and presence

## Goal

Implement authenticated WebSocket signaling and presence fan-out for connected peers.

## Own

- `backend/internal/signaling/`
- Signaling-specific tests.

Do not edit `backend/cmd/server/main.go`, migrations, the auth package, or offline-message/payment packages. Export a handler and hub API for coordinator integration.

## Requirements

- Serve `/v1/ws` and authenticate the JWT using B1's published helper and claims.
- Maintain a concurrency-safe account-to-connection registry.
- Relay `signal` and `ice` payloads without interpreting or persisting SDP.
- Notify only existing connections on peer online/offline transitions.
- Expose `SendTo(accountID, payload)` for transfer notifications. Sending to an offline user must be safe and documented.
- Expose a narrow connect/delivery hook that B4 can use to push pending offline messages. Do not import B4 directly or edit its files.
- Follow `contracts/ws-protocol.md` and the plan for message shapes. Report any mismatch; do not invent a second protocol.

## Dependencies and handoff

- B1 provides the JWT contract.
- B3 provides a connection lookup. Depend on an interface rather than importing ledger internals.
- B4 needs a callback/hook on authenticated connect.
- B5 calls `SendTo` after a committed transfer.
- C2 consumes the WS URL and event shapes.

## Acceptance

- Two test clients can relay a signal and ICE event.
- Online/offline notices target only connected peers.
- `SendTo` with no active recipient does not panic or corrupt hub state.
- Concurrent connect, send, and disconnect tests pass under `go test -race` where feasible.

## Report

Include public constructor and method signatures, auth expectations, WS event examples, offline-delivery hook, and exact test results.
