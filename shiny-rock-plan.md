# Shiny Rock Chat — Project README

A peer‑to‑peer chat application with a supporting backend server. Chats travel peer‑to‑peer over WebRTC whenever possible. The backend provides permanent identity, connection signaling, reliable currency accounting (earn **and** spend), and offline message delivery.

Supported platforms: **Android, iOS, Linux, macOS**. Windows is not a priority.

> **Scope clarifications (v1.3 — mutual scan reward)**
>
> * **Each first-time scan awards exactly 1 shiny-rock to *both* parties.**
>   When A scans B for the first time: **A gets +1** (the scanner) and **B gets +1** (the scanned). Repeat scans by the same scanner award nothing to either party. Enforced at the DB level with `UNIQUE (scanner_id, scanned_id)`.
> * **Shiny-rock transfers (transactions)** are in scope. Once two users are connected, either can send shiny-rock to the other **from inside the chat**. Transfers are atomic, idempotent, require an existing connection, and appear as a special message bubble in both peers' chat timelines.
> * **Global invariant (updated):** `SUM(users.shiny_rock) == 2 × COUNT(scan_events)` at all times, because each scan event mints exactly two shiny-rock.

---

## Table of Contents

1. [Project Summary](#1-project-summary)
2. [Technology Stack](#2-technology-stack)
3. [Architecture Overview](#3-architecture-overview)
4. [Shared Contracts](#4-shared-contracts)
5. [Repository Scaffolding](#5-repository-scaffolding)
6. [Backend Agents](#6-backend-agents)
7. [Client Agents (Flutter)](#7-client-agents-flutter)
8. [Parallelisation Timeline](#8-parallelisation-timeline)
9. [Testing & Integration Checklist](#9-testing--integration-checklist)
10. [Critical Risks & Mitigations](#10-critical-risks--mitigations)
11. [Definition of Done](#11-definition-of-done)

---

## 1. Project Summary

| Feature | Behaviour |
|---|---|
| **Immutable QR identity** | At sign-up, the backend generates or co-signs an asymmetric key pair. The QR code encodes a server-signed payload containing the user's public key fingerprint and a unique `accountId`. The QR is stored server-side and **cannot be changed** after creation. |
| **Scan-to-connect** | Scanning another user's QR initiates a connection. The scanner's app sends the scanned payload to the backend, which validates it, credits **both users +1 shiny-rock** (only on the first scan of that pair), and creates a **connection** (edge in `connections`). The backend then signals both peers via WebSocket to establish a WebRTC data channel. |
| **Shiny-rock currency (earn)** | **Each distinct user who scans you for the first time credits you +1 shiny-rock, and also credits the scanner +1 shiny-rock.** Repeat scans by the same scanner credit nothing to either side. A user's *scan-earned* balance therefore equals `scans_received + scans_made`. |
| **Shiny-rock currency (spend)** | Once two users are **connected**, either can send shiny-rock to the other from inside the chat. Transfers are atomic, idempotent (client-supplied `transferId`), require an existing connection, and appear as a special message bubble in the chat timeline. |
| **Offline delivery** | If B is offline, messages from A are stored encrypted on the backend and pushed to B when it reconnects. Transfers are always accepted by the backend (even if the recipient is offline) and reconciled to the chat timeline on next sync. |

---

## 2. Technology Stack

| Layer | Choice | Why |
|---|---|---|
| **Client UI** | **Flutter** (Dart) | Single codebase for Android, iOS, Linux, macOS. |
| **P2P transport** | **`peer_rtc`** (or `flutter_webrtc`) | Cross-platform WebRTC data channels with built-in signaling, auto-reconnect, and mesh support. |
| **QR generation & image scanning** | **Rust `shared/qr` crate**, exposed through the core API | Generate QR codes and decode QR codes from images across platforms. Evaluate `qrcode` with `rxing` or `rqrr`; check licenses and maintenance in spike S2. |
| **Live camera scanning** | **`mobile_scanner`** on Android, iOS, and macOS | No Linux support. Linux uses image-import scanning through the Rust core API, without another Flutter scanning plugin. |
| **Secure key storage** | **`flutter_secure_storage`** | Keychain (iOS/macOS), Keystore (Android), libsecret (Linux). |
| **Backend language** | **Go** | Native concurrency (goroutines) for signaling and WebSocket fan-out; lower latency than Node.js in WebRTC scenarios. |
| **Backend framework** | **Fiber v2** or **Chi** | Lightweight HTTP + WebSocket handling. |
| **Signaling** | **Custom WebSocket hub** (or adapt `flutter-webrtc-server`) | `flutter-webrtc-server` provides a simple Go signaling server with TURN integration. |
| **Database** | **PostgreSQL** | ACID guarantees for the shiny-rock ledger and transfers. Uses `NUMERIC(20, 8)` for balances so future reward changes and peer transfers don't require a schema migration. |
| **Decimal handling** | **`shopspring/decimal`** (Go), **`decimal`** package (Dart) | Transfers can include fractions; earns are exact integers. Avoid float drift either way. |
| **Message queue (optional)** | **NATS** or **Redis Pub/Sub** | Decouple signaling from ledger updates for horizontal scaling. |

> **Note on reward simplicity:** each first-time scan credits exactly `1.00000000` to *each* party, but balances are still stored as `NUMERIC(20, 8)` so peer transfers can use fractional amounts. Should the reward model ever change, the schema will not need to.

---

## 3. Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                        CLIENT (Flutter)                      │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │ Identity/QR │  │ WebRTC Chat │  │ ShinyRock Wallet UI │  │
│  │             │  │ + Transfers │  │ + Send Shiny-Rock   │  │
│  └──────┬──────┘  └──────┬──────┘  └──────────┬──────────┘  │
└─────────┼────────────────┼────────────────────┼─────────────┘
          │                │                    │
          ▼                ▼                    ▼
┌─────────────────────────────────────────────────────────────┐
│                     BACKEND (Go)                            │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │ Auth & QR   │  │ Signaling   │  │ ShinyRock Ledger    │  │
│  │ Service     │  │ Service     │  │ (earn: +1 both)     │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │ Transfers   │  │ Offline     │  │ Scan + Transfer     │  │
│  │ Service     │  │ Message     │  │ Event Store         │  │
│  │ (spend)     │  │ Queue       │  │ (PostgreSQL)        │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

**Key design decisions:**

1. **The backend never sees plaintext chat messages** when P2P is active. WebRTC data channels are E2E-encrypted by DTLS. Transfer *notifications* flow through the backend, but message content stays P2P.
2. **The QR payload is server-signed.** Even though the client displays the QR, the payload includes a signature from the backend. This prevents forged QR codes.
3. **The ledger is append-only on the earn side.** Each row in `scan_events` represents one first-time scanner pair and corresponds to **two** `+1` credits: one to the scanner, one to the scanned user.
4. **Earn rewards are paid once per `(scanner, scanned)` pair**, enforced by `UNIQUE (scanner_id, scanned_id)`. Each user's `scans_received` = unique scanners who scanned them; `scans_made` = unique users they've scanned. Their total scan-earned balance equals `scans_received + scans_made` (before any transfers).
5. **Idempotency everywhere:**
   - Scans: unique constraint on `(scanner_id, scanned_id)`. Repeat scan → no credit to either side, return `rewarded: false`.
   - Transfers: client-provided UUID `transferId` used as the primary key. Retry with same ID → returns the original transfer, never double-charges.
6. **Transfers require an existing connection** (a `connections` row between the two users). Prevents spam and matches product intent.
7. **Money is stored as `NUMERIC(20, 8)`.** Scan rewards are integers but transfers may be fractional. No `double` anywhere.
8. **Transfers work when the recipient is offline.** The backend accepts and persists the transfer atomically; recipients reconcile via `/v1/payments/history` on next connect.

---

## 4. Shared Contracts

> **Freeze these before parallel work begins.** Any change requires a PR to the `contracts` branch and notification to all agents.

### 4.1 REST API (OpenAPI 3.0)

| Endpoint | Method | Purpose |
|---|---|---|
| `/v1/auth/register` | POST | Create account. Returns `accountId`, `qrPayload` (signed), and a JWT. |
| `/v1/auth/login` | POST | Authenticate and receive JWT. |
| `/v1/account/me` | GET | Return current user profile, balance, `scansReceived`, `scansMade`. |
| `/v1/qr/scan` | POST | Body: `{ "scannedPayload": "..." }`. Validates the scanned QR, **records the scan only if this scanner has never scanned this user before**, credits **+1 shiny-rock to the scanner AND +1 to the scanned user**, and creates a `connections` row. Returns the scan outcome. |
| `/v1/ledger/balance` | GET | Return current user's shiny-rock balance, `scansReceived`, `scansMade`. |
| `/v1/payments/send` | POST | Transfer shiny-rock to a connected peer. Body: `{ "transferId": "<client-uuid-v4>", "recipientId": "...", "amount": "10.5", "note": "optional" }`. Returns the completed transfer and the sender's new balance. |
| `/v1/payments/history` | GET | Query params: `?peerId=<id>&since=<ts>&limit=50`. Returns transfers involving the current user, optionally filtered to a single peer, ordered by `created_at DESC`. |
| `/v1/messages/offline` | GET | Fetch undelivered messages (when peer was offline). |
| `/v1/messages/ack` | POST | Acknowledge delivery of offline messages. |

**`POST /v1/qr/scan` — status codes and reward rule:**

| Status | Meaning |
|---|---|
| `200 OK` | **First-time scan.** Scan event recorded, **scanner credited +1**, **scanned user credited +1**, `connections` row created. Body: `{ "scanIndex": K, "rewarded": true, "alreadyConnected": false }`. |
| `200 OK` (repeat) | **Repeat scan by the same user.** No credit to either side, no new scan event. Body: `{ "rewarded": false, "alreadyConnected": true }`. Existing connection is untouched. **Rewards are strictly one-per-pair.** |
| `400 Bad Request` | Malformed or tampered QR payload. |
| `403 Forbidden` | Attempted to scan own QR. |
| `429 Too Many Requests` | Rate limit hit. |

> **`scanIndex` semantics:** when present, it is the **scanned user's** K-th unique scanner count (`scans_received` after this scan). It is *not* the scanner's count. Both parties receive +1 regardless of `scanIndex`.
>
> **Repeat-scan behaviour:** returns `200 OK` with `rewarded: false` rather than `409` — the user *is* already connected, so they should proceed to the chat; the backend guarantees no second reward.

**`POST /v1/payments/send` — status codes:**

| Status | Meaning |
|---|---|
| `200 OK` | Transfer completed. Body: `{ transfer, senderBalance }`. |
| `400 Bad Request` | Invalid amount (≤ 0, more than 2 decimals), missing fields. |
| `402 Payment Required` | Insufficient balance. |
| `403 Forbidden` | No connection between the two users, or self-transfer attempted. |
| `409 Conflict` | `transferId` already used — returns the **original** transfer for idempotency (do not treat as an error in the client). |
| `429 Too Many Requests` | Rate limit hit. |

### 4.2 WebSocket Protocol (Signaling, Presence & Transfers)

Connect to `wss://<host>/v1/ws?token=<JWT>`.

**Client → Server messages:**

```json
{ "type": "join", "peerId": "<accountId>" }
{ "type": "signal", "to": "<accountId>", "payload": { "sdp": "...", "type": "offer" } }
{ "type": "signal", "to": "<accountId>", "payload": { "sdp": "...", "type": "answer" } }
{ "type": "ice", "to": "<accountId>", "candidate": "..." }
{ "type": "presence", "status": "online" }
```

> **Note:** Transfers are submitted via REST (`/v1/payments/send`), not via WebSocket. The WS channel is used only to *push* transfer notifications.

**Server → Client messages:**

```json
{ "type": "peer-online", "peerId": "<accountId>" }
{ "type": "peer-offline", "peerId": "<accountId>" }
{ "type": "signal", "from": "<accountId>", "payload": { "sdp": "...", "type": "offer" } }
{ "type": "ice", "from": "<accountId>", "candidate": "..." }
{ "type": "offline-message", "message": { "id": "...", "from": "...", "ciphertext": "...", "timestamp": 0 } }
{ "type": "payment-received", "transfer": { "id": "...", "senderId": "...", "recipientId": "...", "amount": "10.5", "note": "", "createdAt": 0 } }
{ "type": "payment-sent",     "transfer": { "id": "...", "senderId": "...", "recipientId": "...", "amount": "10.5", "note": "", "createdAt": 0 } }
```

> **Delivery model:** `payment-received` is pushed to the recipient if online; `payment-sent` is pushed to the sender's other sessions. If a user is offline, no event is queued — the recipient reconciles by calling `GET /v1/payments/history?since=<last-seen-ts>` on WS connect.
>
> **Scan credits are *not* pushed via WebSocket.** The scanner learns of their own +1 synchronously from the `POST /v1/qr/scan` response (`rewarded: true`). The scanned user learns of their +1 by refreshing `/v1/ledger/balance` — the client is expected to refresh after any chat screen open / app foreground / pull-to-refresh.

### 4.3 Shared Data Models

**Dart (client):**

```dart
enum MessageKind { text, paymentSent, paymentReceived }

class ChatMessage {
  final String id;            // for transfers, id == transferId (client-generated UUID)
  final String peerId;
  final MessageKind kind;
  final String? text;         // populated for MessageKind.text
  final String? amount;       // populated for payment kinds, as decimal string
  final int timestamp;
  final bool pending;         // optimistic UI marker
}

class Account {
  final String accountId;
  final String qrPayload;          // signed, immutable
  final String publicKey;          // base64
  final String shinyRockBalance;   // decimal string, e.g. "42.00000000"
  final int scansReceived;         // unique scanners who scanned me
  final int scansMade;             // unique users I have scanned
}

class Transfer {
  final String id;            // transferId (client-generated UUID v4)
  final String senderId;
  final String recipientId;
  final String amount;        // decimal string
  final String? note;
  final int createdAt;
  final String status;        // "completed" | "failed"
}

class Connection {
  final String peerId;        // the other side's accountId
  final int connectedAt;
}

class ScanResult {
  final int? scanIndex;       // present only when rewarded == true; == scanned user's scansReceived
  final bool rewarded;        // true iff BOTH sides were credited +1
  final bool alreadyConnected;
}
```

**Go (backend):**

```go
type Account struct {
    ID            string          `json:"accountId"`
    QRPayload     string          `json:"qrPayload"`
    PublicKey     []byte          `json:"publicKey"`
    ShinyRock     decimal.Decimal `json:"shinyRockBalance"`
    ScansReceived int             `json:"scansReceived"` // unique scanners who scanned me
    ScansMade     int             `json:"scansMade"`     // unique users I have scanned
    CreatedAt     time.Time       `json:"createdAt"`
}

type ScanEvent struct {
    ScannerID string    `json:"scannerId"`
    ScannedID string    `json:"scannedId"`
    ScanIndex int       `json:"scanIndex"` // K for the scanned user
    Timestamp time.Time `json:"timestamp"`
    // Reward columns intentionally omitted — each event mints exactly 1 to each side.
}

type ScanResponse struct {
    ScanIndex        int  `json:"scanIndex,omitempty"`
    Rewarded         bool `json:"rewarded"`         // true iff BOTH sides received +1
    AlreadyConnected bool `json:"alreadyConnected"`
}

type Transfer struct {
    ID          string          `json:"id"`
    SenderID    string          `json:"senderId"`
    RecipientID string          `json:"recipientId"`
    Amount      decimal.Decimal `json:"amount"`
    Note        string          `json:"note,omitempty"`
    Status      string          `json:"status"`
    CreatedAt   time.Time       `json:"createdAt"`
}

type Connection struct {
    UserA     string    `json:"userA"` // lexicographically smaller
    UserB     string    `json:"userB"` // lexicographically larger
    CreatedAt time.Time `json:"createdAt"`
}
```

---

## 5. Repository Scaffolding

Create a monorepo with two top-level directories:

```
srock-chat/
├── backend/          # Go module
├── client/           # Flutter app
├── contracts/        # OpenAPI spec, WebSocket schema, shared models
└── docker-compose.yml
```

Freeze the contracts in `contracts/` and tag the commit `v0-contracts`. All agents branch from this tag.

**Initial scaffold commands:**

```bash
mkdir srock-chat && cd srock-chat
git init

# Backend
mkdir backend && cd backend
go mod init github.com/yourorg/srock-chat/backend
go get github.com/gofiber/fiber/v2
go get github.com/gofiber/websocket/v2
go get github.com/jackc/pgx/v5
go get github.com/golang-jwt/jwt/v5
go get github.com/shopspring/decimal
go get github.com/google/uuid
cd ..

# Client
flutter create --platforms=android,ios,linux,macos client
cd client
flutter pub add peer_rtc mobile_scanner flutter_secure_storage \
                http web_socket_channel decimal uuid share_plus
cd ..

# Contracts
mkdir contracts
# drop openapi.yaml, ws-protocol.md, models.md here
```

---

## 6. Backend Agents

### Agent B1 – Auth & QR Identity Service

**Goal:** Every account gets a permanent, server-signed QR code that cannot be altered.

**Steps:**

1. Create `POST /v1/auth/register`. On success:
   - Generate an ECDSA P-256 key pair on the backend (or accept a client-generated public key and co-sign).
   - Derive `accountId` as `base58(sha256(publicKey))`.
   - Build the QR payload: `srock://v1/<accountId>?pk=<base64(publicKey)>&sig=<serverSignature>`. The signature covers `accountId + publicKey`.
   - Store `accountId`, `publicKey`, `qrPayload`, and hashed password in `users` table.
   - Return a JWT and the `qrPayload`.
2. Create `POST /v1/auth/login` and `GET /v1/account/me`.
3. **Critical:** `qrPayload` is **write-once**. Add a database constraint that prevents updates to `qrPayload` after creation. Never expose an endpoint to regenerate it.
4. **Tests:** Registering twice with the same public key fails; the QR payload signature verifies correctly.

**Files:** `backend/internal/auth/`, `backend/internal/qr/`, `backend/migrations/001_users.sql`.

---

### Agent B2 – WebSocket Signaling & Presence Service

**Goal:** Connect two peers via WebRTC by relaying SDP offers/answers and ICE candidates; push transfer events.

**Steps:**

1. Implement a WebSocket hub in Go (`/v1/ws`). Authenticate via JWT query parameter.
2. Maintain an in-memory map of `accountId → *websocket.Conn`. On connect, send `peer-online` to all **connections** of the connecting user (query `connections` table).
3. Relay `signal` and `ice` messages verbatim between peers. Do not inspect or store SDP payloads.
4. On disconnect, broadcast `peer-offline`.
5. **Expose a `SendTo(accountId, payload)` function** on the hub for other services to call. Agent B5 (Transfers) uses this to push `payment-received` / `payment-sent` events.
6. **Tests:** Two WebSocket clients; a signal sent by one is received by the other. `SendTo` on an offline user is a no-op (no panic).

**Files:** `backend/internal/signaling/hub.go`, `backend/internal/signaling/handler.go`.

---

### Agent B3 – Shiny-Rock Ledger (Earn) + Connections — **+1 to BOTH sides per first-time scan**

**Goal:** Correct, idempotent, atomic accounting of shiny-rock scan rewards — **exactly +1 to the scanner and +1 to the scanned user, once per `(scanner, scanned)` pair** — plus the `connections` table that gates transfers.

**Steps:**

1. Create `scan_events` and `connections` tables. **No `reward` column** — a scan event *is* two mints:

   ```sql
   CREATE TABLE scan_events (
     id BIGSERIAL PRIMARY KEY,
     scanner_id TEXT NOT NULL,
     scanned_id TEXT NOT NULL,
     scan_index INT NOT NULL,
     created_at TIMESTAMPTZ DEFAULT now(),
     UNIQUE (scanner_id, scanned_id)   -- ⬅ ONE reward per pair, enforced by DB
   );

   CREATE TABLE connections (
     user_a TEXT NOT NULL REFERENCES users(account_id),
     user_b TEXT NOT NULL REFERENCES users(account_id),
     created_at TIMESTAMPTZ DEFAULT now(),
     PRIMARY KEY (user_a, user_b),
     CHECK (user_a < user_b)           -- canonical order
   );
   ```

   Add two integer columns to `users`:

   ```sql
   ALTER TABLE users ADD COLUMN scans_received INT NOT NULL DEFAULT 0;
   ALTER TABLE users ADD COLUMN scans_made     INT NOT NULL DEFAULT 0;
   -- If a legacy total_scans column exists, migrate: scans_received = total_scans, scans_made = 0.
   ```

2. Implement `POST /v1/qr/scan`:
   - Validate the scanned `qrPayload` signature. Reject self-scan with `403`.
   - Begin a transaction.
   - **First-time-scan guard:**
     ```sql
     SELECT EXISTS (
       SELECT 1 FROM scan_events
       WHERE scanner_id = $scanner AND scanned_id = $scanned
     ) AS already_scanned;
     ```
     *If `already_scanned == true`:*
     - Do **not** insert into `scan_events`.
     - Do **not** credit either side.
     - Return `200` with `{ rewarded: false, alreadyConnected: true }`.
   - If this is the first scan by this scanner:
     - `scanIndex = (SELECT COUNT(*) FROM scan_events WHERE scanned_id = ?) + 1`
       — the **scanned** user's unique-scanner count after this scan.
     - `INSERT INTO scan_events (scanner_id, scanned_id, scan_index) VALUES (...)`
       — if this insert ever hits the unique constraint (concurrent duplicate scan), roll back and return the same `{ rewarded: false, alreadyConnected: true }` response.
     - **Credit the scanned user:** `UPDATE users SET shiny_rock = shiny_rock + 1, scans_received = scans_received + 1 WHERE account_id = $scanned`.
     - **Credit the scanner:** `UPDATE users SET shiny_rock = shiny_rock + 1, scans_made = scans_made + 1 WHERE account_id = $scanner`.
     - `INSERT INTO connections (user_a, user_b) VALUES (LEAST, GREATEST) ON CONFLICT DO NOTHING`.
     - Return `200` with `{ rewarded: true, scanIndex, alreadyConnected: false }`.
   - Commit.
3. Add `GET /v1/ledger/balance` returning `{ shinyRockBalance, scansReceived, scansMade }`.
4. Expose `ConnectionsExist(a, b) bool` helper (used by B5's transfer handler).
5. **Tests — must all pass:**
   - **A scans B once** → A's balance **+1**, B's balance **+1**, `scan_events` has 1 row, `connections` has 1 row.
   - **A scans B again** → both balances **unchanged**, `scan_events` still 1 row, response says `rewarded: false`.
   - **A scans B 100 times in parallel** → exactly **+1 to each** total.
   - **A, C, D each scan B once** → B's balance **+3**, A/C/D each **+1**, `scan_events` has 3 rows.
   - **B scans A (back) after A scanned B** → new row `(B, A)`; **both +1 again**. This is intentional — each directional first-scan is independent.
   - **Concurrency across different scanners:** 100 parallel first-time scans to the same user → 100 unique `scan_index` values; the scanned user gains +100; the 100 scanners each gain +1.
   - **`scan_index` monotonicity** per scanned user: never skips or duplicates.
   - **Per-user invariant:** for every user, `balance_ignoring_transfers == scans_received + scans_made`.
   - **Global invariant:** `SELECT SUM(shiny_rock) FROM users == 2 * SELECT COUNT(*) FROM scan_events`.

**Files:** `backend/internal/ledger/service.go`, `backend/internal/ledger/connections.go`, `backend/migrations/002_scan_events.sql`, `backend/migrations/003_connections.sql`, `backend/migrations/003b_user_scan_counters.sql`.

---

### Agent B4 – Offline Message Queue

**Goal:** Deliver messages reliably when one or both peers are offline.

**Steps:**

1. Create `offline_messages` table:

   ```sql
   CREATE TABLE offline_messages (
     id UUID PRIMARY KEY,
     sender_id TEXT NOT NULL,
     recipient_id TEXT NOT NULL,
     ciphertext BYTEA NOT NULL,    -- E2E encrypted by client
     created_at TIMESTAMPTZ DEFAULT now(),
     delivered_at TIMESTAMPTZ
   );
   ```

2. When the signaling service detects that the recipient is offline, the client calls `POST /v1/messages/offline`.
3. When a user connects via WebSocket, query undelivered messages and push them as `offline-message` events.
4. Provide `POST /v1/messages/ack`.
5. **Tests:** Store a message for an offline user, connect that user via WebSocket, assert the message is pushed.

> **Note:** Transfers do **not** use this queue. The backend persists them in `transfers` atomically; clients reconcile via `/v1/payments/history` on connect.

**Files:** `backend/internal/messages/offline.go`, `backend/migrations/004_offline_messages.sql`.

---

### Agent B5 – Transfers Service (Spend)

**Goal:** Allow a connected user to transfer shiny-rock to a peer, atomically, idempotently, and with proper authorization.

**Steps:**

1. Create `transfers` table:

   ```sql
   CREATE TABLE transfers (
     id UUID PRIMARY KEY,              -- client-supplied transferId (idempotency)
     sender_id TEXT NOT NULL REFERENCES users(account_id),
     recipient_id TEXT NOT NULL REFERENCES users(account_id),
     amount NUMERIC(20, 8) NOT NULL CHECK (amount > 0),
     note TEXT,
     status TEXT NOT NULL DEFAULT 'completed'
       CHECK (status IN ('completed', 'failed')),
     created_at TIMESTAMPTZ DEFAULT now()
   );

   CREATE INDEX idx_transfers_sender    ON transfers(sender_id, created_at DESC);
   CREATE INDEX idx_transfers_recipient ON transfers(recipient_id, created_at DESC);
   CREATE INDEX idx_transfers_pair
     ON transfers(LEAST(sender_id, recipient_id),
                  GREATEST(sender_id, recipient_id),
                  created_at DESC);
   ```

2. Implement `POST /v1/payments/send`:
   - **Validate body** (`transferId` is valid UUID v4; `recipientId` != `senderId`; `amount > 0`; at most 2 decimal places; fits `NUMERIC(20, 8)`).
   - **Idempotency check (before transaction):**
     - `SELECT * FROM transfers WHERE id = transferId`.
     - If found and `sender_id = current_user`, return `200` with the existing transfer + current balance (retries are safe).
     - If found and `sender_id != current_user`, return `403`.
   - **Authorization:** `SELECT EXISTS (... FROM connections WHERE user_a = LEAST(a,b) AND user_b = GREATEST(a,b))`. Not connected → `403`.
   - **Rate limit:** Per-sender token bucket (30 transfers/min) → `429`.
   - **Transaction:**
     ```sql
     BEGIN;
     SELECT shiny_rock FROM users WHERE account_id = $sender FOR UPDATE;
     -- if balance < amount: ROLLBACK; return 402
     UPDATE users SET shiny_rock = shiny_rock - $amount WHERE account_id = $sender;
     UPDATE users SET shiny_rock = shiny_rock + $amount WHERE account_id = $recipient;
     INSERT INTO transfers (id, sender_id, recipient_id, amount, note, status)
       VALUES (...) ON CONFLICT (id) DO NOTHING;
     COMMIT;
     ```
   - **Post-commit notify:** Use the signaling hub (B2) to push:
     - `payment-received` → recipient (if online).
     - `payment-sent` → sender (other sessions).
   - **Return** `{ transfer, senderBalance }`.

3. Implement `GET /v1/payments/history`:
   - Params: optional `peerId`, optional `since` (unix ms), optional `limit` (default 50, max 200).
   - Use the pair index when `peerId` is set.
   - Order by `created_at DESC`.

4. **Invariants to enforce / test:**
   - **Global mint invariant:** `SUM(users.shiny_rock) == 2 × COUNT(scan_events)`. Every scan event mints exactly 2; transfers are zero-sum.
   - A user can never go negative.
   - No duplicate `transferId` produces two balance changes.

5. **Tests:**
   - Happy path (A sends 10 to B), insufficient balance (402), not connected (403), self-transfer (403), duplicate `transferId` (200 with original), concurrent duplicate `transferId` (exactly one debit), rate limit (429).

**Files:** `backend/internal/payments/service.go`, `backend/internal/payments/handler.go`, `backend/migrations/005_transfers.sql`.

---

## 7. Client Agents (Flutter)

### Agent C1 – Identity, Secure Storage & QR Display

**Goal:** Securely store keys and render the immutable QR code.

**Steps:**

1. On first launch, call `POST /v1/auth/register`. Store the JWT and the server-signed `qrPayload` in `flutter_secure_storage`.
2. Render the QR through the Rust `shared/qr` crate exposed by the core API. The QR content is **exactly** the `qrPayload` returned by the backend — never regenerate locally.
3. Add a "Share" button (`share_plus`).
4. **Tests:** The stored `qrPayload` is retrieved unchanged on second launch; the core API renders that exact payload as a QR image.

**Files:** `client/lib/identity/`, `client/lib/ui/qr_display.dart`.

---

### Agent C2 – WebRTC Chat & Signaling Client

**Goal:** Establish P2P chat channels using the backend's signaling service; ingest transfer notifications.

**Steps:**

1. Connect to `wss://<host>/v1/ws?token=<JWT>` on app start.
2. When the user scans a QR (C3), call `POST /v1/qr/scan`, then send a `signal` (offer) to the scanned peer via WebSocket.
3. Use `peer_rtc` (or `flutter_webrtc`) to create a `Peer` instance. On receiving a `signal`, set the remote description and create an answer.
4. Once the data channel is open, send messages as JSON `{ "type": "chat", "text": "..." }`.
5. Listen for `offline-message` WS events and display them in the chat list.
6. **Listen for transfer events on the WS stream:**
   - `payment-received` → `chatStore.addPaymentMessage(peerId: transfer.senderId, transfer: ...)`.
   - `payment-sent` → `chatStore.addPaymentMessage(peerId: transfer.recipientId, transfer: ...)`.
   These are idempotent upserts by `transferId`, so duplicates from reconnect are harmless.
7. On WS **connect**, call `GET /v1/payments/history?since=<last_seen_ts>` and store the `last_seen_ts` locally. This reconciles transfers that happened while offline.
8. **Tests:** Mock WebSocket server emits `payment-received`; assert a payment message is added to the local chat store exactly once (idempotent across reconnects).

**Files:** `client/lib/webrtc/`, `client/lib/chat/chat_service.dart`, `client/lib/chat/payment_sync.dart`.

---

### Agent C3 – QR Scanner & Scan Handler

**Goal:** Scan QR codes and trigger the backend scan event and P2P connection. Handle the "first-scan-only reward" contract gracefully — repeat scans still navigate to chat, they just don't reward.

**Steps:**

1. Use `mobile_scanner` for live camera scanning on Android, iOS, and macOS.
   Linux users import an image; decode its QR through the Rust `shared/qr`
   crate exposed by the core API.
2. Decode imported QR images through the Rust `shared/qr` crate exposed by the core API.
3. On detection, extract the `qrPayload`. Call `POST /v1/qr/scan`.
4. On `200` (whether `rewarded: true` or `false`):
   - **Always** initiate the WebRTC connection (C2) and navigate to chat.
   - **If `rewarded: true`:** the scanner has just earned **+1 shiny-rock** (as has the scanned user). Immediately refresh the local wallet balance so the scanner sees the credit. Optionally show a brief non-intrusive toast like "+1 shiny-rock earned".
   - **If `rewarded: false`:** no toast — the pair is already connected.
5. On `400` / `403`, show a user-friendly error.
6. **Tests:** A mocked camera or imported image emits a known payload; `POST /v1/qr/scan` is called. A second scan for the same payload still navigates to chat but does not trigger a balance refresh or toast.

**Files:** `client/lib/ui/qr_scanner.dart`, `client/lib/scan/scan_handler.dart`.

---

### Agent C4 – Chat UI & Wallet Display

**Goal:** Display chat history (including transfer bubbles), connection status, and shiny-rock balance.

**Steps:**

1. Build `ChatScreen(peerId)` that listens to a merged message stream from `chatStore` and renders:
   - `MessageKind.text` → normal bubble.
   - `MessageKind.paymentSent` / `MessageKind.paymentReceived` → transfer bubble with amount, direction (sent/received), and timestamp. Pending state (`pending: true`) shows a spinner; confirmed shows a checkmark.
2. Show a "reconnecting" indicator when the WebSocket or WebRTC connection drops.
3. Build a `WalletScreen` that calls `GET /v1/ledger/balance` and displays shiny-rock with 2 decimal places. Because earns are whole numbers, users will see integers for scan-earned balance and fractions only when a transfer has handed them a fractional amount.
   - **Wallet refresh triggers:** app foreground, `ChatScreen` first open, pull-to-refresh, after a successful scan (`rewarded: true`), and after a successful transfer. This ensures the scanned user's **+1** (which is not pushed via WebSocket) is visible promptly.
4. **Tests:** Widget test with a mock message stream containing both text and payment messages; both render correctly and transfer bubbles show the right direction. Widget test that calling `wallet.refresh()` updates the displayed balance.

**Files:** `client/lib/ui/chat/`, `client/lib/ui/chat/payment_bubble.dart`, `client/lib/ui/wallet/`.

---

### Agent C5 – Platform Build & CI

**Goal:** One-click builds for Android, iOS, Linux, macOS.

**Steps:**

1. **Android:** `CAMERA`, `INTERNET` permissions. `minSdkVersion 23`.
2. **iOS:** `NSCameraUsageDescription`. Keychain Sharing capability.
3. **macOS:** Keychain Sharing and camera entitlements.
4. **Linux:** Document `libsecret` dependency.
5. GitHub Actions matrix for `android`, `ios`, `linux`, `macos`.
6. **Tests:** CI must pass `flutter test` and a smoke build per platform.

**Files:** `.github/workflows/`, `client/android/`, `client/ios/`, `client/macos/`, `client/linux/`.

---

### Agent C6 – Transfers UI (Send Shiny-Rock in Chat)

**Goal:** Let a connected user send shiny-rock to their peer from inside the chat, with clear validation, optimistic feedback, and idempotent retries.

**Steps:**

1. **Entry point:** Add a "Send shiny-rock" button in the `ChatScreen` app bar or input bar. Enable only when a connection to this peer exists.
2. **Dialog:** Modal with:
   - Amount `TextField` (`numberWithOptions(decimal: true)`).
   - Optional note `TextField` (max 140 chars).
   - Live validation: `> 0`, `≤ currentBalance`, at most 2 decimal places.
   - Confirm button.
3. **On confirm:**
   - Generate `transferId = Uuid().v4()` **before** the request.
   - **Optimistically** append `ChatMessage(kind: paymentSent, id: transferId, pending: true, amount: ...)`.
   - Call `POST /v1/payments/send`.
4. **On response:**
   - `200`: update message `pending: false`; update wallet balance from `senderBalance`; success snackbar.
   - `402`: remove optimistic message; "Insufficient shiny-rock".
   - `403`: remove optimistic message; "You can only pay connected users" / "Cannot pay yourself".
   - `409`: treat as success — the transfer already exists. Update local message to non-pending using the returned `transfer`.
   - **Network error:** keep the optimistic message `pending: true`; queue a retry (exponential backoff). On retry success, follow the `200` path. **Never** generate a new `transferId` for a retry.
5. **Store:** `PaymentRepository` with `send(...)` and `history({peerId, since})`. Local `payments` table keyed by `transferId` (upsert).
6. **Reconciliation on chat open:** Call `GET /v1/payments/history?peerId=<peer>` and merge results via idempotent upsert on `transferId`.
7. **Tests:**
   - Valid amount → endpoint called with `transferId` matching the optimistic message; message becomes non-pending.
   - Insufficient balance → optimistic message removed; error shown.
   - Retry uses the **same** `transferId` across attempts.
   - `history(peerId: X)` merges into local store without duplicating existing entries.

**Files:** `client/lib/payments/payment_repository.dart`, `client/lib/payments/send_payment_dialog.dart`, `client/lib/ui/chat/send_payment_button.dart`.

---

## 8. Parallelisation Timeline

| Week | Backend Agents | Client Agents |
|---|---|---|
| **1** | B1 (Auth/QR) + B3 (Ledger + Connections, +1 to both) + B4 (Offline Queue) start in parallel. B2 (Signaling) starts after B1 provides JWT shape. | C1 (Identity/QR) + C5 (Build) start. C3 (Scanner) can start with a mock backend. |
| **2** | B2 finishes WebSocket hub (including `SendTo`). **B5 (Transfers) starts once `connections` schema from B3 is frozen.** | C2 (WebRTC + transfer events) integrates with B2. **C6 (Transfers UI) starts against a mocked `/v1/payments/*` API using the frozen contract.** C4 (UI) builds screens. |
| **3** | Integration: register → scan → both get +1 → repeat scan (no credit) → send transfer → confirm balance. Load test on `/v1/qr/scan` and `/v1/payments/send` for concurrency. | End-to-end test on Android + iOS: scan QR, both balances +1, connect, send message, **send and receive shiny-rock, verify balance and chat bubble**. |

---

## 9. Testing & Integration Checklist

**Earn (scans) — +1 to both sides per first-time scanner:**

- [ ] **QR immutability:** Attempt to update `qrPayload` via any API → must fail.
- [ ] **First scan credits both:** A scans B for the first time → A's balance **+1** AND B's balance **+1**.
- [ ] **Repeat scan — no reward:** A scans B a second time → **both** balances unchanged, `scan_events` still has 1 row for `(A, B)`, response has `rewarded: false`.
- [ ] **Repeat scan — many times:** A scans B 1000 times → both balances after the 1st scan are identical to after the 1000th.
- [ ] **Concurrent duplicate scan:** 100 parallel scans of B by A → exactly **+1 to each** total.
- [ ] **Unique scanner count:** A, C, D each scan B once → B's balance **+3**, A/C/D each **+1**, `scan_events` has 3 rows.
- [ ] **Mutual scan:** A scans B (both +1), then B scans A (both +1 again) → both users end at +2.
- [ ] **Concurrent unique scans:** 100 parallel first-time scans by 100 distinct scanners to B → 100 unique `scan_index` values; B gains **+100**; each scanner gains **+1**.
- [ ] **Per-user invariant:** `balance_ignoring_transfers == scans_received + scans_made`.
- [ ] **Global invariant:** `SUM(users.shiny_rock) == 2 × COUNT(scan_events)`.

**Spend (transfers):**

- [ ] **Happy path:** A → B sends 10; A −10, B +10.
- [ ] **Idempotency:** Same `transferId` twice → only one debit/credit.
- [ ] **Concurrent duplicate:** 100 parallel POSTs with the same `transferId` → exactly one debit.
- [ ] **Insufficient balance:** 402, no balance change.
- [ ] **Not connected:** 403.
- [ ] **Self-transfer:** 403.
- [ ] **Rate limit:** 31 rapid transfers → 429 on the last one.
- [ ] **Recipient offline:** Transfer accepted, B reconciles via `/v1/payments/history` on reconnect.
- [ ] **Retry safety:** Network drops mid-request; retry with same `transferId` → one debit only.
- [ ] **Global mint invariant:** `SUM(users.shiny_rock) == 2 × COUNT(scan_events)`. Transfers are zero-sum.

**Delivery & security:**

- [ ] **Offline message delivery:** A sends a message while B is offline; B receives it on reconnect.
- [ ] **P2P fallback:** WebRTC failure routes messages through the offline queue.
- [ ] **Secure storage:** JWT / private key survive restart; not readable in plaintext on disk.
- [ ] **Cross-platform QR:** QR from Android is scannable by iOS, Linux, macOS.
- [ ] **Transfer event deduplication:** Receiving the same `payment-received` twice (reconnect) results in one chat bubble (upsert on `transferId`).

---

## 10. Critical Risks & Mitigations

| Risk | Mitigation |
|---|---|
| **QR code forgery** | Server-signs the payload; client verifies before processing a scan. |
| **Repeat-scan reward farming** | `UNIQUE (scanner_id, scanned_id)` at the DB level; `POST /v1/qr/scan` short-circuits to `rewarded: false` on repeat scans; concurrency-safe via `INSERT ... ON CONFLICT DO NOTHING` and a rolled-back transaction on conflict. |
| **Scan race conditions** | PostgreSQL transaction + unique constraint. |
| **Self-scan farming** | `POST /v1/qr/scan` rejects `scanner_id == scanned_id` with `403`. |
| **Transfer double-spend** | `SELECT ... FOR UPDATE` on sender row inside transaction. |
| **Duplicate transfer from retry** | Client-generated `transferId` as PK; second request returns the original transfer. |
| **Unauthorized transfers** | `connections` table check; no connection → 403. |
| **Float drift in transfers** | `NUMERIC(20, 8)` in DB; `shopspring/decimal` (Go); `decimal` (Dart). Never use `double`. |
| **Transfer spam / probing** | Per-sender token bucket (30/min) → 429. |
| **Transfer delivered while P2P is down** | Transfers are REST operations, not P2P. Chat timeline reconciles via `/v1/payments/history`. |
| **WebRTC NAT traversal failure** | Bundle TURN server (coturn). |
| **Client-side key compromise** | `flutter_secure_storage` with biometric protection on Android/iOS. |
| **Signaling server bottleneck** | Go goroutines; add Redis Pub/Sub for horizontal scale. |
| **Replay attacks on scan endpoint** | Server-issued nonce in QR payload; reject scans older than a configurable TTL. |
| **Abuse (fake accounts scanning)** | Rate-limit scans per IP and per account; require email verification before first scan counts. |
| **Transfer notification lost while offline** | Client fetches `/v1/payments/history?since=<ts>` on every WS connect. |
| **Scanned user's +1 not pushed via WS** | Client refreshes wallet on app foreground, chat open, pull-to-refresh, and after any successful scan/transfer. Documented as part of C4. |

---

## 11. Definition of Done

The project is complete when:

1. Two users on different platforms (e.g. Android + macOS) can register, scan each other's QR codes, and exchange messages over WebRTC.
2. **A first-time scan credits +1 shiny-rock to the scanner AND +1 to the scanned user**, verified by an integration test.
3. **Scanning the same user a second time does not credit either party — the reward is strictly one-per-pair — and the client still navigates to the chat.**
4. A connected user can send shiny-rock to their peer from inside the chat; the sender's balance decreases and the recipient's increases atomically, in exactly one ledger event.
5. A transfer appears as a distinct message bubble (sent/received direction) in both peers' chat timelines, even if one peer was offline at send time.
6. Retrying a failed send with the same `transferId` never double-charges.
7. Unconnected users cannot send shiny-rock to each other.
8. If a chat message is sent while the recipient is offline, it is delivered on reconnect.
9. All platform builds (Android, iOS, Linux, macOS) pass CI.
10. The QR code displayed in the app is byte-identical to the one stored on the backend and cannot be changed.
11. **Global invariant holds at all times:** `SUM(users.shiny_rock) == 2 × COUNT(scan_events)`.