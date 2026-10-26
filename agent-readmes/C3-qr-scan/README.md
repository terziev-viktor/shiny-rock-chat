# C3 — QR scanner and scan handler

## Goal

Scan/import a user's QR payload, record the scan, and open the peer chat whether the scan is new or repeated.

## Own

- `client/lib/scan/`
- `client/lib/ui/qr_scanner.dart`
- Scanner and scan-handler tests.

Do not edit identity display, WebRTC implementation, chat-store internals, wallet widgets, or platform configuration.

## Requirements

- Use `mobile_scanner` for live camera scan on Android, iOS, and macOS.
- Support Linux image import through the shared QR decode adapter.
- Call `POST /v1/qr/scan` with the decoded payload.
- On either successful result (`rewarded: true` or `false`), open/connect to the peer and navigate to chat.
- Refresh the scanner's wallet and show an optional reward toast only when `rewarded` is true.
- Show clear errors for invalid payloads and rejected self-scans.
- Inject camera, image decoder, API, navigation, and wallet dependencies so tests need no camera.
- Consume the agreed QR adapter from C1 or the coordinator. Do not create another QR bridge.

## Dependencies and handoff

- B3 provides scan request/response behavior.
- C1/coordinator provides QR decode adapter.
- C2 provides the connect/open-peer entry point.
- C4 provides wallet refresh API.

## Acceptance

- A valid payload calls the scan API and navigates to chat.
- Repeated scans still navigate but do not refresh balance or show reward feedback.
- Invalid/self-scan failures show user-facing errors.
- Tests cover camera and imported-image results through mocks.

## Report

Include scan API adapter, navigation/callback contract, QR decoder dependency, and exact test results.
