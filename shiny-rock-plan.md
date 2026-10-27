# Shiny-Rock Chat — Agent README

A peer-to-peer chat with a backend server. Every account owns one permanent QR code. You connect with someone by scanning their QR. Each time your QR is scanned by a **new** person you earn **shiny-rock** (the K-th rewarded scan of your QR is worth **N / K**). Once two users are connected, either can **send shiny-rock to the other inside the chat**.

Clients: **Android and Linux**. Apple platforms (iOS, macOS) are **out of scope** because they cannot be served without proprietary dependencies. Windows is not a priority.
Server: one backend that handles accounts, the authoritative ledger (scan rewards and transfers), peer signaling, offline mailbox and push.
**Everything we ship and depend on is FOSS. Where no FOSS option exists, we write our own library.**

This file is the single source of truth for how agents split the work. Read it fully before touching code.

---

## 1. Invariants (non-negotiable)

| # | Invariant |
|---|-----------|
| I1 | Creating an account creates a server-assigned random `account_id` and binds it to the client's Ed25519 public key. The QR encodes `account_id` + key fingerprint. |
| I2 | The QR never changes. No rotate/re-issue/regenerate endpoint exists. The DB enforces it (immutable columns, UPDATE blocked by trigger). |
| I3 | Two accounts become contacts only after one scans the other's QR and the scanned side confirms (co-signed handshake recorded by the server). No search/discovery by phone, email or name. |
| I4 | A rewarded scan of user S's QR credits **S** (the scanned user), not the scanner. |
| I5 | The K-th rewarded scan of S is worth `N / K` (K starts at 1). `N` is undecided: it lives in exactly one versioned config value, never hardcoded. |
| I6 | **A scan pair is rewarded once.** For the ordered pair `(scanner → scanned)` only the first confirmed scan is rewarded. If A scans B twice, B is rewarded only for the first scan. Repeats, and self-scans, do not increment K, do not credit anything, and return the original receipt (idempotent). The pair `(B → A)` is a different pair and is rewarded separately. |
| I7 | Messages are end-to-end encrypted. The server never sees plaintext message content. |
| I8 | Money is integer fixed-point (µ-rock, 1 shiny-rock = 1,000,000 µ-rock). No floats anywhere in ledger code. |
| I9 | The ledger is append-only and double-entry. Rows are never updated or deleted. Each row stores its computed amount at write time, so changing `N` later never rewrites history. |
| I10 | **Transfers**: only between two accounts that are contacts. Sender must be authenticated and a signed `TransferIntent` authorizes it. Amount > 0, sender ≠ recipient, no overdraft: balance can never go below zero. |
| I11 | A transfer is atomic (debit and credit in one DB transaction), idempotent (same idempotency key returns the same result, never a second transfer) and irreversible (no refund/undo in v1). |
| I12 | **Conservation**: total supply = sum of all scan rewards ever issued. Transfers only move existing value. Checked by an automated invariant job. |
| I13 | The server is the only authority for balances and transfer outcomes. A transfer "message" in a chat is only a reference to a server-issued `TransferReceipt`; clients never trust a peer's claim that a transfer happened. |
| I14 | **FOSS only.** Every library, service, tool and build step in the shipped app and the server must be under an OSI-approved license, with source available. No proprietary SDKs or services (no Firebase/FCM, no Google Play Services, no Play Integrity, no APNs/Apple frameworks, no closed-source blobs). If no FOSS option exists, we implement our own (§8 rule 13). |

### Scan reward math (spec for `shared/reward`)

- The K-th rewarded scan of account S credits `floor(N / K)` µ-rock. Rounding: floor, remainder discarded (documented, tested).
- After K rewarded scans S has earned `Σ floor(N / i)` for i = 1..K (about `N · H_K`, the harmonic series).
- `K` is the per-account counter of rewarded scans, incremented atomically in the same DB transaction that writes the ledger row. Two simultaneous scans must never get the same K. A duplicate pair never reaches the counter (rejected by `UNIQUE(scanner_id, scanned_id)`, then handled as idempotent replay).
- Golden vectors in `/spec/vectors/reward/`, plus property tests (non-increasing values, determinism, no overflow up to K = 10^9).

### Transfer rules (spec for `shared/ledger-rules`)

- Valid iff: sender and recipient are contacts, `amount_micro >= MIN_TRANSFER`, `amount_micro <= available balance`, within configured limits (D-8), idempotency key unused, signature valid, nonce fresh.
- No fees in v1.
- Validation helpers are shared by server and client so the client can pre-check, but the server's result is final.

---

## 2. Open decisions (human owns these). Code against the default, isolate it behind an interface, record an ADR.

| ID | Decision | Default |
|----|----------|---------|
| D-1 | Value of `N` | placeholder `REWARD_N = 100_000_000` µ-rock in a versioned config epoch table; changing it creates a new epoch and affects only future scans |
| D-2 | Spending beyond transfers (shop, conversion to real money) | Out of scope. Shiny-rock is non-convertible in v1. If it ever becomes convertible to money, get legal advice first |
| D-3 | Account recovery | BIP39-style recovery phrase that re-derives the identity key. Same account, same QR, same balance |
| D-4 | Anti-farming (no device attestation is possible under I14) | Per-pair single reward (I6), registration **proof-of-work** (own `shared/pow` crate), per-IP/per-device-secret rate limits, new-account cooldown (D-8). Optional later: in-person proximity challenge. Decide before M4 |
| D-5 | P2P transport | WebRTC data channels with server signaling and a coturn TURN fallback. `flutter_webrtc` is used **directly** (BSD) and plugged into a Rust `Transport` trait (WP-A3, WP-D7). Confirmed or replaced by spike WP-S2 |
| D-6 | Client stack | Flutter (BSD) UI over a shared Rust core |
| D-7 | Server stack | Rust (axum) + PostgreSQL + **Valkey** (not Redis), containerized; infra as plain Compose/Kubernetes manifests or **OpenTofu** (not Terraform) |
| D-8 | Transfer limits | `MIN_TRANSFER` = 1 µ-rock, per-transfer max and daily cap per sender in config, 24 h cooldown before a brand-new account may send. All values are config only |
| D-9 | Optional memo on a transfer | Allowed, plain text, max 140 chars, E2E-encrypted inside the chat message only |
| D-10 | QR scanning/rendering | No Google ML Kit. Camera frames come from the Flutter `camera` plugin (Android uses CameraX, Apache-2.0; spike S2 verifies it needs no Google Play Services) and are decoded in the Rust `shared/qr` crate (`rxing` or `rqrr`, license checked). QR drawing also in `shared/qr` (`qrcode` crate) rendered by a Flutter `CustomPainter`. Linux v1 scans by image import/paste only. If the `camera` plugin fails the spike, we write a thin own Android camera plugin (§8 rule 13) |
| D-11 | Project license | Not chosen yet. Candidates: AGPL-3.0 (strong copyleft, protects the server), MPL-2.0, Apache-2.0. Default for now: **AGPL-3.0-or-later** for everything; the choice constrains which dependency licenses are allowed (WP-F7 allow-list follows this decision) |
| D-12 | Push notifications (no FCM/APNs) | **UnifiedPush** (FOSS, Web Push standard RFC 8030 with VAPID) via a distributor app such as ntfy. Fallback for users without a distributor: an Android foreground service holding a WebSocket to the server (costs battery) |
| D-13 | Distribution | F-Droid, a self-hosted F-Droid repository, and direct signed APK downloads. No Google Play. Linux: AppImage/Flatpak/.deb. Goal: reproducible builds |
| D-14 | Android build toolchain purity | The Android SDK platform/build tools are distributed under Google's SDK license (source is AOSP/Apache-2.0, binaries from Google). Default: use the command-line SDK tools and accept this build-time exception; strict alternative: use distro-built/rebuilt SDK packages (e.g. Debian's, as F-Droid-style builds do). Same question for the Flutter SDK (BSD, but downloads prebuilt engine binaries from Google): default accept, with a documented path to build the engine from source |

---

## 3. Architecture

```
┌──────────── Clients (Android, Linux): Flutter UI over Rust core ────────────┐
│ onboarding │ my QR │ scan │ chats (+send shiny-rock) │ wallet │ settings    │
└───────▲──────────────────────────────▲──────────────────────────────────────┘
        │ WebRTC data channel (P2P)    │ HTTPS + WebSocket (+ UnifiedPush in)
        │ (TURN relay if NAT blocks)   ▼
        │                 ┌─────────────────────── Server ───────────────────────┐
        ▼                 │ api-gateway (auth, rate limit, PoW check)              │
   other client           │ account-svc  ledger-svc  transfer-svc  signaling-svc   │
                          │ mailbox-svc  push-gw (Web Push/UnifiedPush) coturn      │
                          └─ PostgreSQL (accounts, scans, ledger, transfers) + Valkey ┘
```

Behaviour: when both peers are online they chat directly (WebRTC). Otherwise the sender uploads an **encrypted** blob to the mailbox and the recipient is woken via UnifiedPush (or the WebSocket service). Money never moves peer-to-peer: scan rewards and transfers are always executed by the server ledger, and the chat only carries a reference to the resulting receipt.

### Repository layout (monorepo)

```
/spec/             proto schemas, OpenAPI, QR format, protocol docs, test vectors   (Track S)
/server/           Rust workspace: services below                                   (Track B)
  crates/api  crates/account  crates/ledger  crates/transfer
  crates/signaling crates/mailbox  crates/push  crates/common
/core/             client Rust workspace                                            (Track A)
  crates/identity crates/storage crates/p2p crates/session crates/handshake
  crates/wallet crates/sync crates/ffi
/shared/           used by BOTH server and client: reward, ledger-rules, protocol, qr, pow   (Track S)
/app/              Flutter app + Android and Linux runners                          (Tracks C, D, E)
/infra/            compose, manifests/OpenTofu, coturn config, CI                   (Track G)
/tools/            multi-client simulator, load tests, fixtures, license audit      (Track F)
/docs/adr/         decision records
```

### Server data model (sketch, owned by WP-B1)

```
accounts(account_id PK, pubkey, key_fp, created_at, qr_payload, pow_proof)   -- immutable columns
devices(device_id PK, account_id FK, push_endpoint, platform, created_at)
scans(scan_id PK, scanner_id FK, scanned_id FK, scanner_sig, scanned_sig,
      created_at, UNIQUE(scanner_id, scanned_id))                            -- I6
scan_counters(account_id PK, k BIGINT)                                       -- locked per update
contacts(a_id, b_id, scan_id, created_at)
transfers(transfer_id PK, from_id FK, to_id FK, amount_micro BIGINT CHECK (>0),
          idempotency_key, sender_sig, created_at,
          UNIQUE(from_id, idempotency_key))                                  -- I11
ledger_entries(seq BIGSERIAL PK, account_id FK,
               kind ENUM('scan_reward','transfer_out','transfer_in'),
               amount_micro BIGINT,          -- signed: + credit, - debit
               scan_id FK NULL, transfer_id FK NULL, k BIGINT NULL,
               config_epoch INT NULL, created_at)                            -- append-only
account_balances(account_id PK, balance_micro BIGINT CHECK (>= 0))           -- updated in same tx
mailbox(msg_id PK, recipient_id, blob BYTEA, created_at, expires_at)
config_epochs(epoch PK, reward_n BIGINT, active_from)
```

Invariant job: `SUM(ledger_entries.amount_micro) = SUM(account_balances) = SUM(scan_reward rows)`.

### Scan transaction (WP-B3)

1. Scanner submits `ScanIntent` (signed, idempotency key) to `POST /v1/scans`.
2. Server validates: signature, scanner ≠ scanned, rate limits. If the pair already exists, return the original receipt and stop (I6).
3. Server notifies the scanned user; their app returns a signed `ScanAck`.
4. ONE DB transaction: insert `scans`, `contacts`; lock `scan_counters` row; `k += 1`; `amount = floor(N_active / k)`; insert `ledger_entries(kind=scan_reward)`; update `account_balances`.
5. Return co-signed `ScanReceipt` to both; the wallet updates via event.

### Transfer transaction (WP-B8)

1. In a chat, the sender enters an amount (and optional memo). The client builds `TransferIntent {to, amount_micro, idempotency_key, nonce, timestamp}` and signs it with the identity key.
2. `POST /v1/transfers`. Server validates signature, nonce freshness, contact relationship, limits (D-8), rate limits.
3. ONE DB transaction: lock both `account_balances` rows **in ascending `account_id` order** (prevents deadlocks); check `balance >= amount`; insert `transfers`; insert two `ledger_entries`; update both balances.
4. Server returns a server-signed `TransferReceipt {transfer_id, from, to, amount, seq, timestamp}` and pushes a `TransferReceived` event to the recipient.
5. The sender's client posts a **`transfer` chat message** (E2E, carrying `transfer_id`, optional memo). Both clients render the bubble by fetching/verifying the `TransferReceipt` from the server (I13), never from the peer's message alone.
6. Retries with the same idempotency key return the same receipt.

### Registration and proof-of-work (WP-B2, WP-S4)

Account creation requires a valid proof-of-work solution (own `shared/pow` crate, memory-hard such as Argon2-based, difficulty set by server config and raised under load) bound to the new public key and a server-issued nonce. This makes mass fake-account creation expensive without any proprietary attestation. It is mitigation, not proof of humanity; see risks.

---

## 4. Contracts (WP-S1 produces these FIRST; everything depends on them)

1. **QR payload**: `shinyrock://v1/<account_id>.<key_fp>` + checksum, with test vectors.
2. **Protobuf** (`/spec/proto`): `Account`, `PowChallenge`, `PowSolution`, `ScanIntent`, `ScanAck`, `ScanReceipt`, `TransferIntent`, `TransferReceipt`, `LedgerEntry`, `Envelope` (E2E message kinds `text` and `transfer`), `MailboxItem`, `SignalingMessage`, `Event` (`ScanRewarded`, `TransferReceived`, `BalanceChanged`).
3. **Server API** (`/spec/server.openapi.yaml` + WebSocket protocol doc): PoW challenge, accounts, scans, transfers (`POST /v1/transfers`, `GET /v1/transfers/{id}`), ledger/transactions query with pagination, balance, mailbox, signaling, push registration (UnifiedPush endpoint + VAPID key), auth (challenge-response signed by the identity key; short-lived tokens).
4. **Client core API** (`/spec/core-api.md`): `create_account`, `get_qr`, `begin_scan`, `confirm_scan`, `list_contacts`, `send_message`, `send_shiny_rock(contact, amount, memo?)`, `get_balance`, `list_transactions`, `subscribe_events`, `export_recovery`, `restore_account`, `render_qr_matrix(payload)`, `decode_qr_image(bytes)`, `decode_qr_frame(luma_frame)`.
5. **Reward, ledger-rules and PoW specs + vectors** (§1, §3).
6. **Chat extension point**: a `MessageRenderer` registry so new message kinds (`transfer`) plug into the chat UI without editing the chat feature's files.

Rule: **a contract change is a PR to `/spec` only**, reviewed before dependents continue. Everyone regenerates code from `/spec`; never hand-edit generated code.

---

## 5. Work packages

`deps` must be merged first. Each package lists **owned paths**; do not edit paths you don't own.

### Phase 0: Foundation

| ID | Package | Owns | Deps | Done when |
|----|---------|------|------|-----------|
| WP-00 | Monorepo scaffold, lint/format, CI skeleton on **FOSS CI** (self-hosted Forgejo/Woodpecker or GitLab CE; decision recorded as ADR). Linux runners only. | `/`, `infra/ci` | none | CI green on empty workspaces; **Android APK build and Linux build of the empty Flutter app run in CI from day 1** |
| WP-S1 | **Contracts** from §4 with test vectors | `/spec` | none | Tagged `spec-v0.1` |
| WP-S2 | Spikes → ADRs: (a) `flutter_webrtc` on Android and Linux, NAT/background behavior, TURN; (b) E2E library choice, license check (vodozemac Apache-2.0 / OpenMLS MIT preferred; libsignal is AGPL and only acceptable if D-11 is AGPL-compatible); (c) verify `camera` plugin on Android works **without Google Play Services** (test on a de-Googled image) and that `flutter_webrtc` does too, Android `compileSdk`/AGP/Gradle/Kotlin pinned in the app; (d) key storage: Android Keystore (AOSP) and libsecret on Linux; (e) UnifiedPush end to end with a FOSS distributor and a Web Push/VAPID sender; (f) Rust QR decode quality (`rxing` vs `rqrr`) | `/docs/adr` | none | One ADR each with a recommendation |
| WP-S3 | `shared/reward` (N/K math, once-per-pair helpers) and `shared/ledger-rules` (transfer validation), fixed-point, vectors, property tests | `shared/reward`, `shared/ledger-rules` | S1 | Used by server and client; all tests pass |
| WP-S4 | **Own library `shared/pow`**: memory-hard proof-of-work (generate challenge, solve, verify), constant-time verify, tunable difficulty, test vectors, benchmarks on a low-end Android phone | `shared/pow` | S1 | Solve time within target on reference phone; verify under 5 ms on server |

### Track B: Server (parallel after `spec-v0.1`)

| ID | Package | Owns | Deps | Acceptance |
|----|---------|------|------|------------|
| WP-B1 | Schema + migrations + repositories, immutability and append-only triggers (I2, I9), balance CHECK constraints | `server/crates/common`, `server/migrations` | S1 | UPDATE/DELETE on immutable/append-only tables fails; migrations up/down tested |
| WP-B2 | **account-svc**: PoW-gated registration (key binding, `account_id` + QR issue), challenge-response auth, tokens, recovery re-bind | `server/crates/account` | B1, S4 | No endpoint can change `qr_payload`; invalid/replayed PoW rejected |
| WP-B3 | **ledger-svc**: double-entry posting primitives (`post_entries` atomically updating balances, lock ordering), scan transaction (§3), balance/history queries | `server/crates/ledger` | B1, S3 | 1,000 parallel scans of one account get distinct K 1..1000; **A scans B twice → B credited once, second call returns the original receipt**; self-scan rejected; conservation holds |
| WP-B4 | **signaling-svc**: WebSocket presence + WebRTC offer/answer/ICE relay, contact-gated | `server/crates/signaling` | B1, B2 | Non-contacts refused |
| WP-B5 | **mailbox-svc**: encrypted blob store-and-forward, TTL, quotas, ack/delete | `server/crates/mailbox` | B1, B2 | Opaque bytes only; expiry and quota tests |
| WP-B6 | **push-gw**: Web Push (RFC 8030 + VAPID) sender to UnifiedPush endpoints; opaque wake-ups (no content, no amounts, no sender identity); also drives the WebSocket fallback | `server/crates/push` | B1, B2 | Payload audit test; delivers through a FOSS distributor in compose |
| WP-B7 | **api-gateway**: routing, rate limiting, abuse controls, PoW difficulty control, observability (Prometheus/Grafana/Loki, all FOSS) | `server/crates/api` | B2 | Rate-limit and PoW-difficulty tests |
| WP-B8 | **transfer-svc**: transfer transaction (§3), idempotency, nonce/replay protection, limits (D-8), `TransferReceipt` signing, events. Develops against the `post_entries` trait from B3 (mock first) | `server/crates/transfer` | B1, B2, S3, B3 trait | 100 parallel transfers from one account with balance B never overdraw; A→B and B→A concurrently do not deadlock; same idempotency key twice = one transfer; non-contact refused; conservation holds |

### Track A: Client core (parallel with B; develops against mocks generated from OpenAPI)

| ID | Package | Owns | Deps | Acceptance |
|----|---------|------|------|------------|
| WP-A1 | Identity + `KeyStore` trait, recovery-phrase derive/export, QR payload encode/decode, intent signing (`ScanIntent`, `TransferIntent`) | `core/crates/identity` | S1 | Vectors pass; identity immutable |
| WP-A2 | Encrypted local DB (SQLCipher community edition, BSD-style), migrations | `core/crates/storage` | S1 | Encrypted at rest verified |
| WP-A3 | P2P layer in Rust: `Transport` trait (open/close peer channel, send/receive framed bytes), signaling client, connection state machine, reconnect/backoff, framing, in-memory loopback transport | `core/crates/p2p` | S1 | Core end-to-end tests pass over loopback; real WebRTC plugged in by WP-D7 |
| WP-A4 | E2E session + message protocol (ratchet, ordering, receipts, `text` and `transfer` kinds) | `core/crates/session` | S1, S2(b) | Offline/out-of-order and forward-secrecy tests |
| WP-A5 | Scan handshake client side (`ScanIntent`, `ScanAck`, receipt verification) | `core/crates/handshake` | A1, S1 | Forged/replayed receipts rejected |
| WP-A6 | Sync + offline queue (mailbox fetch, retries, dedupe) | `core/crates/sync` | A2, A4, B5 mocks | Flaky-network test passes |
| WP-A7 | Core façade implementing `/spec/core-api.md` | `core/crates/ffi` (Rust side) | A1..A6, A8, A9 (mocks first) | API conformance tests |
| WP-A8 | **Wallet**: build/sign `TransferIntent`, pre-validation with `shared/ledger-rules`, idempotent retry queue, `TransferReceipt` verification, local balance/transaction cache | `core/crates/wallet` | A1, A2, S3, B8 mocks | Crash mid-send then restart produces exactly one transfer; tampered receipt rejected; peer-claimed transfer without valid receipt flagged |
| WP-A9 | **QR crate** `shared/qr`: QR matrix generation, decoding from image bytes and from camera luma frames, FFI-friendly API | `shared/qr` | S1, S2(f) | Round-trips spec vectors; decodes rotated, low-contrast, compressed-screenshot QR images and live-frame fixtures |

### Track C: Bindings

| ID | Package | Owns | Deps | Acceptance |
|----|---------|------|------|------------|
| WP-C1 | FFI bridge (flutter_rust_bridge/UniFFI) + native build scripts: Android ABIs (arm64-v8a, armeabi-v7a, x86_64) and Linux (x86_64, aarch64) | `app/bridge`, `tools/build-native` | S1, A7 skeleton | Hello-world call on Android and Linux in CI |

### Track D: Flutter UI (develops against a mock core)

| ID | Package | Owns | Deps | Acceptance |
|----|---------|------|------|------------|
| WP-D0 | App shell, theme, navigation, mock core (incl. mock wallet), `MessageRenderer` registry | `app/lib/shell`, `app/lib/mock` | S1 | Runs fully on mocks |
| WP-D1 | Onboarding (incl. PoW progress screen), account creation, "My QR" screen (painter from `render_qr_matrix`, share as image) | `app/lib/features/onboarding` | D0, S4 | QR matches spec vectors |
| WP-D2 | QR scanner: Android live camera (`camera` plugin frames → `decode_qr_frame`); Linux and fallback everywhere: image import/paste → `decode_qr_image`. Shows "already connected, no new reward" for repeat scans | `app/lib/features/scan` | D0, A9 mock, S2(c) | Scan → handshake progress → success/failure/already-scanned; Linux works with image import |
| WP-D3 | Chat list + conversation (text, status, timestamps) | `app/lib/features/chat` | D0 | Online/offline states on mock |
| WP-D4 | Wallet: balance, transaction history (scan rewards with K and amount, sent, received) | `app/lib/features/wallet` | D0, S3 | µ-rock formatted correctly; pagination |
| WP-D5 | Settings, recovery phrase export/restore with strong warnings (I2), push mode (UnifiedPush / background connection) | `app/lib/features/settings` | D0 | Round-trip on mock |
| WP-D6 | **Send shiny-rock in chat**: send action, amount entry with balance check, confirmation, in-flight/success/failure states, `transfer` bubble via `MessageRenderer` ("verified" only after receipt check), optional memo | `app/lib/features/chat_transfer` | D0, A8 mocks | Works on mocks incl. insufficient balance, retry after network loss, duplicate-tap safe |
| WP-D7 | **WebRTC adapter**: implements the core `Transport` trait with `flutter_webrtc` directly (data channels, ICE/TURN config from server, state events, bytes bridged to the core). No wrapper packages | `app/lib/transport_webrtc` | D0, A3 trait, S2(a) | Two app instances exchange framed bytes via server signaling and TURN in compose; reconnects after network switch |

### Track E: Platform integration (each owns its runner folder)

| ID | Package | Owns | Deps | Acceptance |
|----|---------|------|------|------------|
| WP-E1 | **Android**: camera permission, **own thin Kotlin `KeyStore` implementation on the AOSP Android Keystore**, UnifiedPush client integration (FOSS `unifiedpush` library) with foreground-service WebSocket fallback, ABIs, F-Droid-compatible build (no proprietary deps, no anti-features), reproducible APK signing | `app/android` | C1 | Receives a message and a transfer notification while backgrounded on a device **without Google Play Services**; F-Droid metadata builds from source |
| WP-E2 | **Linux**: libsecret `KeyStore` with encrypted-file fallback, desktop notifications over the live connection, packaging (AppImage, Flatpak, .deb) | `app/linux` | C1 | Runs on Ubuntu LTS + Fedora; no proprietary runtime parts |

### Track F: Quality and security (start early, never blocks)

| ID | Package | Owns | Deps | Acceptance |
|----|---------|------|------|------------|
| WP-F1 | Multi-client simulator + fixtures (NAT, latency, loss profiles) | `tools/sim` | A3 draft | Used by A3, A4, A6 tests |
| WP-F2 | Threat model + review: reward farming without attestation (PoW economics, sybil scans), QR replay, transfer replay/double-spend, forged receipts, laundering across fake accounts, token theft, metadata leakage, malicious server | `docs/security` | S1 | Findings filed as `security` issues |
| WP-F3 | End-to-end tests: scan → chat → balance → send shiny-rock → both balances correct; Linux↔Linux in CI, Android on real devices or an AOSP-based emulator image (decision recorded in ADR) | `tools/e2e` | D*, E*, G1 | Nightly CI |
| WP-F4 | Load tests: scan transaction, transfer transaction under contention (hot account), signaling | `tools/load` | B3, B8 | Documented capacity numbers |
| WP-F5 | Docs and privacy policy draft (server sees: who scanned whom, who paid whom and how much, when, online presence) | `docs` | none | Human review |
| WP-F6 | **Ledger audit tool**: replays `ledger_entries` from scratch, verifies balances, conservation (I12) and once-per-pair (I6); CI and production cron | `tools/ledger-audit` | B3 | Detects a deliberately corrupted row in a test |
| WP-F7 | **License and SBOM audit (enforces I14)**: `cargo-deny`, Dart/Flutter and Gradle dependency license scan, REUSE compliance, SBOM generation, allow-list derived from D-11, proprietary-blob scanner for the APK (e.g. flags `com.google.android.gms`, Firebase, ML Kit classes); CI fails on any violation | `tools/license-audit` | WP-00 | A test PR adding a proprietary or incompatible dependency fails CI |

### Track G: Infra

| ID | Package | Owns | Deps | Acceptance |
|----|---------|------|------|------------|
| WP-G1 | docker compose (or podman compose) for the whole server stack incl. a UnifiedPush distributor, seed data | `infra/compose` | B1 stubs | One command starts everything locally |
| WP-G2 | coturn TURN deployment and credential scheme | `infra/turn` | S1 | Relay works across strict NATs |
| WP-G3 | Production deploy on a Linux VPS, secrets, **PostgreSQL backups with point-in-time recovery (the ledger is money)**, observability, migration pipeline, TLS via a FOSS ACME client | `infra/prod` | B* | Staging live; restore drill documented |

---

## 6. Dependency graph and parallel lanes

```
WP-00 ─┐
WP-S1 ─┼─► [spec-v0.1] ─┬─► S3 ─────────► B3 ─► B8, F6
WP-S2 ─┘                ├─► S4 ─► B2 (PoW) ; D1
                        ├─► B1 ─► B2 ─► B4 B5 B6 B7
                        ├─► A1 ► A5 ┐
                        ├─► A2 ► A6 ┤
                        ├─► A3      ├─► A7 ─► C1 ─► E1 E2 ─► F3
                        ├─► A4      │
                        ├─► A8 ─────┤   (A8 starts on mocks of B8)
                        ├─► A9 ─────┘
                        ├─► D0 ─► D1 D2 D3 D4 D5 D6 D7   (mocks; wire to A7 later)
                        ├─► G1 G2 ─► G3
                        └─► F1 F2 F5 F7
```

Parallelism right after `spec-v0.1`: S3, S4, B1, A1–A4, A8 (mocks), A9, D0, G1, G2, F1, F2, F5, F7 all at once. Then B2 → B4–B7, B3 → B8, D1–D7, A5–A7 fan out.

Suggested agents (adjust to availability):
1. Spec agent: S1, reviews contract PRs
2. Spike agent: S2
3. Rewards/ledger-rules agent: S3, then B3
4. Server-data agent: B1, B2
5. Server-money agent: B8, F6 (works closely with agent 3)
6. Server-realtime agent: B4, B5, B6
7. Gateway/abuse agent: B7, F4
8. Crypto/identity agent: A1, A5, S4
9. Storage + sync agent: A2, A6
10. Network agent: A3, F1, G2
11. Messaging agent: A4
12. Wallet agent: A8
13. QR/vision agent: A9
14. Bindings agent: A7, C1
15. UI agents (2–3): D0–D7
16. Platform agents: E1 (Android), E2 (Linux)
17. Infra agent: G1, G3, WP-00
18. QA/security/compliance agent: F2, F3, F5, F7

---

## 7. Milestones

| M | Goal | Includes |
|---|------|----------|
| M0 | Contracts frozen, FOSS CI up | 00, S1, S2, F7 |
| M1 | Server walking skeleton: PoW-gated register, QR, scan with correct N/K and once-per-pair, transfers between contacts, signaling, mailbox, ledger audit passing, in compose, driven by a CLI test client | S3, S4, B1–B5, B8, F6, G1 |
| M2 | Linux client: account, QR, scan via image import, chat, wallet, send shiny-rock in chat | A1–A9, C1, D0–D4, D6, D7, E2 |
| M3 | Android client: camera scan, UnifiedPush + fallback, offline delivery, F-Droid build, works without Google Play Services | E1, B6, B7, D5 |
| M4 | Hardening: anti-farming review, double-spend and replay review, security review, recovery flow, load tests, production deploy with backups | F2, F3, F4, G3 |
| M5 | Beta | all |

---

## 8. Rules for agents

1. **Stay inside your owned paths.** Cross-boundary needs go through `/spec` or an issue.
2. **Contracts first.** If one is missing or ambiguous, propose it in `/spec`, then continue behind a feature flag.
3. **Branches:** `wp/<id>-<short-name>`; small PRs; rebase on `main` before opening.
4. **Definition of done:** acceptance met, tests, docs, CI green (including the license audit WP-F7), no TODO without an issue number.
5. **Secrets:** none in the repo. Private identity keys never leave the `KeyStore` implementation. Never log keys, tokens, plaintext, amounts tied to identities, or message metadata outside debug builds.
6. **Determinism:** reward and ledger code is integer-only and identical across platforms; server and client use the same `shared/*` crates.
7. **Server authority (I13):** clients may display balances and transfers but only the server decides them. Never trust a client-computed K, amount, balance, or a peer-claimed transfer.
8. **Money code review:** any change to `server/crates/ledger`, `server/crates/transfer`, `shared/reward` or `shared/ledger-rules` needs a second reviewer and must keep the conservation and concurrency tests green.
9. **Mocks are first-class:** every module exposes a trait plus an in-memory mock so others can work in parallel.
10. **Status:** one line per WP in `docs/STATUS.md` (not started / in progress / blocked-on / done).
11. **Do not decide D-1…D-14 yourself.** Use the default, isolate it, write an ADR.
12. **FOSS dependency policy (I14).** Every new Flutter, Rust, Kotlin or server dependency needs an ADR line covering: OSI-approved license compatible with D-11, source available, verified maintainers, release age of at least two weeks with real adoption, no proprietary transitive dependencies, and a green build on Android and Linux in CI. Pin exact versions. Use the upstream library directly instead of wrappers or forks that re-wrap something we already use (this is why `flutter_webrtc_zxing` is rejected). Prebuilt binaries are accepted only if their source is FOSS, buildable by us, and the checksum is pinned (for example the libwebrtc build inside `flutter_webrtc`). Platform build settings (`compileSdk`, `minSdk`, AGP, Gradle, Kotlin) are set only in `app/android`. If a dependency breaks a build or violates I14, replace it rather than patching it, unless the fix is trivial.
13. **Build-our-own rule.** If no FOSS option exists for a need, do not use a proprietary one. Open an ADR "build-own" with: the exact requirement, why existing options fail, the smallest useful scope, an owner WP, a spec in `/spec`, tests/vectors, and the license (D-11). Keep our own libraries small and single-purpose. Current build-own items: `shared/pow` (S4), the Android Keystore bridge in `app/android` (E1), and, only if the spike fails, a thin Android camera plugin (D-10). Candidates to evaluate later: live webcam scanning on Linux.

---

## 9. Key risks

- **Reward farming without attestation:** there is no proprietary device attestation, so the defenses are weaker: per-pair single reward (I6), PoW at registration (S4), rate limits, new-account cooldown, optional proximity challenge (D-4). `N/K` makes the first scans most valuable, so test this specifically (F2). Be ready to tune PoW difficulty (B7).
- **Laundering through transfers:** a farmer can pool rewards from many fake accounts into one. Transfers don't create value (I12), but this matters if shiny-rock ever gains real-world value. Mitigations: cooldown and daily caps (D-8), contact-only transfers, audit tool (F6), review (F2).
- **Double-spend / race conditions:** only the single DB transaction with ordered row locks plus CHECK constraints protects us; keep it simple and heavily tested (B8, F4).
- **Irreversible transfers:** a mistaken amount cannot be undone in v1. UX must show a clear confirmation (D6).
- **Lost key = lost identity and balance**, because the QR is permanent. Recovery phrase UX (D5) is critical.
- **Push on Android without Google:** UnifiedPush needs the user to install a distributor; the foreground-service fallback costs battery. Reliability must be measured on real devices (E1, F3).
- **Android is not fully FOSS at the platform level:** most phones ship Google's proprietary firmware and services, and the SDK has license caveats (D-14). We only control our own app and build: it must run correctly without any proprietary Google component.
- **Smaller audience:** no iOS or macOS, no Google Play. Distribution is F-Droid, own repo or APK (D-13).
- **Linux has no live camera scanning in v1:** image import only (D-10).
- **Metadata honesty:** the server necessarily sees who scanned whom and who paid whom and how much. Say so in privacy docs (F5).
- **Legal:** while shiny-rock is non-convertible and non-purchasable (D-2) this stays simple. Any path to real money changes that.
- **Changing N later** must create a new config epoch and never alter existing ledger rows (I9).
- **License drift:** transitive dependencies can bring in non-FOSS code. WP-F7 must run on every PR.
