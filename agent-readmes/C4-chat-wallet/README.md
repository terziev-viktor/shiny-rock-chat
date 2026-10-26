# C4 — Chat UI and wallet

## Goal

Render text and payment messages, connection state, and the current shiny-rock balance.

## Own

- `client/lib/ui/chat/` except standalone payment-send components owned by C6.
- `client/lib/ui/wallet/`
- Chat and wallet widget tests.

You own `ChatScreen` and its integration points. Do not implement payment sending or edit chat-store internals.

## Requirements

- Render text, sent-payment, and received-payment messages from C2's public chat-store stream.
- Show transfer amount, direction, timestamp, and pending/confirmed state.
- Show reconnecting state when WebSocket or WebRTC is unavailable.
- Load wallet balance from `GET /v1/ledger/balance`.
- Refresh wallet on app foreground, opening ChatScreen, pull-to-refresh, a successful rewarded scan, and successful transfer.
- Expose a narrow integration point for C6's send-payment widget. C6 must not edit `ChatScreen`; provide the callback/slot details in your report.
- Format decimal balances without using floating-point conversion.

## Dependencies and handoff

- C2 provides message models, stream, and connection status.
- B3 provides the balance endpoint contract.
- C3 and C6 need the wallet refresh operation.
- C6 supplies the send-payment widget for coordinator integration.

## Acceptance

- Widget tests render text and both payment directions correctly.
- Pending and confirmed states are distinguishable.
- Wallet refresh updates the displayed value from a mock API.
- Refresh triggers are testable and do not create duplicate requests unnecessarily.

## Report

Include ChatScreen integration point, wallet refresh API, message rendering assumptions, and exact test results.
