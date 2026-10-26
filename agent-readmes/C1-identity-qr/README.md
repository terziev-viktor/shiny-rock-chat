# C1 — Identity, secure storage, and QR display

## Goal

Register a client identity, store credentials securely, and display the backend-issued immutable QR payload.

## Own

- `client/lib/identity/`
- `client/lib/ui/qr_display.dart`
- Focused identity/storage/widget tests.

Do not edit scanner flow, shared chat models, native platform configuration, or `pubspec.yaml` without coordinator approval.

## Requirements

- Call B1's registration/login API and store the JWT and exact server-issued `qrPayload` in `flutter_secure_storage`.
- Never generate or modify the QR identity payload locally.
- Render and share the exact stored payload. Use the existing shared QR core API when present.
- Keep network, secure-storage, and QR rendering dependencies injectable for tests.
- The scaffold currently has no Rust `shared/qr` crate. Do not invent native bindings or a competing QR implementation. Define or consume the agreed adapter boundary and report any missing core dependency.

## Dependencies and handoff

- B1 provides registration response and payload format.
- C3 consumes the shared QR decode boundary if needed.
- C5 owns platform-specific secure storage configuration.

## Acceptance

- A stored payload is retrieved unchanged after app restart.
- QR rendering receives the exact backend payload.
- Share action uses the same payload.
- Tests cover storage and failure paths without requiring a device.

## Report

Include API adapter signatures, secure-storage keys, QR boundary assumptions, and exact test results.
