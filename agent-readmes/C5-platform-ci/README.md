# C5 — Platform build and CI

## Goal

Configure required platform permissions and credentials, document Linux system packages, and add Flutter CI coverage.

## Own

- `.github/workflows/`
- Platform configuration under `client/android/` and `client/linux/`
- Platform-specific build documentation, if required.

Do not edit Flutter feature code, tests, or dependency manifests without coordinator approval.

## Requirements

- Android: camera and internet permissions; minimum SDK 23.
- Linux: document the `libsecret` build/runtime dependency.
- Add CI for Android and Linux with `flutter test` and platform smoke builds.
- Keep workflow secrets out of repository files.
- Do not claim unsupported platform builds passed. Record runner limitations and use appropriate conditional jobs.

## Dependencies and handoff

- Feature agents may identify platform plugin requirements. Ask the coordinator before expanding scope or changing `pubspec.yaml`.
- CI should validate the final merged feature code; the coordinator may need to update workflows after integration.

## Acceptance

- Native permissions and entitlements match the plan.
- CI runs tests and Android/Linux smoke builds with clear platform-specific results.
- Linux system dependency is documented.

## Report

Include changed platforms/workflows, exact local checks, CI limitations, and any required repository secrets or settings (names only, never values).
