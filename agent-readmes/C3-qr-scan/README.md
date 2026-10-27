# C3 — QR scanner and scan handler

## Goal

Scan/import a user's QR payload, record the scan, and open the peer chat whether the scan is new or repeated.

## Own

- `client/lib/scan/`
- `client/lib/ui/qr_scanner.dart`
- Scanner and scan-handler tests.

Do not edit identity display, WebRTC implementation, chat-store internals, wallet widgets, or platform configuration.

## Requirements

- Capture live camera frames on Android with the Flutter `camera` plugin; decode frames through Rust `shared/qr` via `decode_qr_frame`.
- Support Linux image import through `shared/qr` via `decode_qr_image`.
- Call `POST /v1/qr/scan` with the decoded payload.
- On either successful result (`rewarded: true` or `false`), open/connect to the peer and navigate to chat.
- Refresh the scanner's wallet and show an optional reward toast only when `rewarded` is true.
- Show clear errors for invalid payloads and rejected self-scans.
- Inject camera, QR core API, server API, navigation, and wallet dependencies so tests need no camera.
- Use the FOSS QR decoding crate selected by the plan's spike (`rxing` or `rqrr`, license verified under I14). Do not use Google ML Kit or add a competing QR bridge.

## Dependencies and handoff

- B3 provides scan request/response behavior.
- WP-A9/C1 provides the `shared/qr` Flutter/core API.
- C2 provides the connect/open-peer entry point.
- C4 provides wallet refresh API.

## Acceptance

- A valid payload calls the scan API and navigates to chat.
- Repeated scans still navigate but do not refresh balance or show reward feedback.
- Invalid/self-scan failures show user-facing errors.
- Tests cover camera-frame and imported-image decoding through mocks, plus scan API behavior.

## Report

Include scan API adapter, navigation/callback contract, selected FOSS QR decoder and license, and exact test results.
