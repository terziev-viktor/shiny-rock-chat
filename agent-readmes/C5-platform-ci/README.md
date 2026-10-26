# C5 — Platform build and CI

## Goal

Configure required platform permissions and credentials, document Linux system packages, and add Flutter CI coverage.

## Own

- `.github/workflows/`
- Platform configuration under `client/android/`, `client/ios/`, `client/linux/`, and `client/macos/`
- Platform-specific build documentation, if required.

Do not edit Flutter feature code, tests, or dependency manifests without coordinator approval.

## Requirements

- Android: camera and internet permissions; minimum SDK 23.
- iOS: camera usage description and required keychain capability.
- macOS: camera and keychain entitlements.
- Linux: document the `libsecret` build/runtime dependency.
- Add CI for Android, iOS, Linux, and macOS with `flutter test` and platform smoke builds where runners support them.
- Keep workflow secrets out of repository files.
- Do not claim unsupported platform builds passed. Record runner limitations and use appropriate conditional jobs.

## Dependencies and handoff

- Feature agents may identify platform plugin requirements. Ask the coordinator before expanding scope or changing `pubspec.yaml`.
- CI should validate the final merged feature code; the coordinator may need to update workflows after integration.

## Acceptance

- Native permissions and entitlements match the plan.
- CI runs tests and configured smoke builds with clear platform-specific results.
- Linux system dependency is documented.

## Report

Include changed platforms/workflows, exact local checks, CI limitations, and any required repository secrets or settings (names only, never values).
