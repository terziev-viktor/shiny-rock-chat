# Agent work briefs

These briefs split the implementation in [`../shiny-rock-plan.md`](../shiny-rock-plan.md) into independently reviewable tasks. Each agent should read this index, its own brief, and the shared plan before changing code.

## Parallel work rules

1. Give each agent a separate branch or worktree from the same frozen baseline. Use `agent/<ID>-<short-name>` branch names. Do not have agents edit the same checkout.
2. Do not merge or cherry-pick another agent's work into an active task branch. Report the branch and commit for coordinator review and integration.
3. Treat `shiny-rock-plan.md` as the feature source of truth. Do not edit `contracts/` in parallel. Report contract gaps or contradictions to the coordinator.
4. Own only the files listed in the task brief. Do not edit `backend/cmd/server/main.go`, shared route registration, dependency manifests, or another agent's files without coordinator approval.
5. Keep cross-agent calls behind interfaces or constructors. Share the exact public signatures and JSON shape in the report before dependent agents integrate.
6. Do not silently create replacement migrations, duplicate models, or alternate APIs when a dependency is missing. Use a mock or interface for independent work, then report the dependency.
7. Run focused tests and formatting for owned code. Do not claim a platform or database test passed unless it ran successfully.

If the `v0-contracts` tag is not present, stop before branching and ask the coordinator to identify the frozen baseline. Do not create or move the tag yourself.

## Ownership and order

| Agent | Owns | Depends on |
|---|---|---|
| [B1 — Auth and QR identity](B1-auth-qr/README.md) | `backend/internal/auth/`, `backend/internal/qr/`, migration `001` | None |
| [B2 — Signaling and presence](B2-signaling/README.md) | `backend/internal/signaling/` | B1 JWT contract; B3 connections query |
| [B3 — Ledger and connections](B3-ledger/README.md) | `backend/internal/ledger/`, migrations `002`, `003`, `003b` | B1 users schema |
| [B4 — Offline message queue](B4-offline-messages/README.md) | `backend/internal/messages/`, migration `004` | B1 users schema; B2 delivery hook |
| [B5 — Transfers service](B5-transfers/README.md) | `backend/internal/payments/`, migration `005` | B2 `SendTo`; B3 connection and balance APIs |
| [C1 — Identity and QR display](C1-identity-qr/README.md) | `client/lib/identity/`, QR display widget | B1 auth response; QR core API |
| [C2 — WebRTC and signaling client](C2-webrtc-chat/README.md) | `client/lib/webrtc/`, chat transport and sync | B2 WS protocol; shared chat-store API |
| [C3 — QR scanner and scan handler](C3-qr-scan/README.md) | Scanner and scan-handler files | B3 scan API; C2 connect entry point; QR decode API |
| [C4 — Chat UI and wallet](C4-chat-wallet/README.md) | Chat and wallet presentation files | C2 message-store API; B3 balance API |
| [C5 — Platform build and CI](C5-platform-ci/README.md) | Platform configuration and `.github/workflows/` | Existing Flutter plugin requirements |
| [C6 — Transfers UI](C6-transfers-ui/README.md) | Payment repository and send-payment components | B5 payment API; C2 chat-store API; C4 integration point |

## Integration boundaries

- B1 owns the initial `users` schema and migration `001`. B3, B4, and B5 reference it; they must not redefine the base user table.
- B3 owns scan counters and `scan_events`/`connections`. B5 consumes the connection-check API and must not edit B3 files or migrations.
- B2 owns the WebSocket hub. B4 requests an offline-delivery hook through an interface; B5 uses the published `SendTo` API. Neither changes B2 files.
- C2 owns chat message models and the store API. C4 renders that API; C6 adds payments through it without changing C2's implementation.
- C4 owns `ChatScreen`. C6 supplies a standalone send-payment widget and an integration note; the coordinator wires it into `ChatScreen` after review.
- C1 owns the shared Flutter QR adapter boundary if one is needed. C3 consumes it and must not create a competing QR bridge. The Rust `shared/qr` core is not present in the current scaffold; report that dependency if it remains unavailable.
- C5 owns native platform files and CI only. Do not edit Flutter feature code or `pubspec.yaml` unless the coordinator assigns that change.

## Agent report format

Each agent reports:

- Branch and commit hash.
- Files changed and public interfaces added.
- Tests, formatting, and builds run, with exact results.
- Dependencies or contract decisions still needed.
- Any deviations from the plan and why.

Coordinator reviews each report and diff, resolves cross-agent integration, runs integration tests, then merges. Agents do not self-merge.
