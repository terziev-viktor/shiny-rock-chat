# Shiny Rock Chat

Monorepo scaffold for the Flutter client, Go backend, and shared contracts.

## Development environment

Open this repository in VS Code and run **Dev Containers: Reopen in Container**.
The container provides Flutter/Dart, Go, Flutter Linux build libraries, and Android
SDK tooling. Docker Compose starts PostgreSQL and a local coturn service.

PostgreSQL is available at `db:5432` from the devcontainer and `localhost:5432`
from the host. Development credentials are `shinyrock` / `shinyrock-dev` for the
`shinyrock` database; `DATABASE_URL` is set in the workspace container. The coturn
service uses the local-only development credential `dev` / `dev-only-change-me`,
available in the container through `TURN_URL`, `TURN_USERNAME`, and `TURN_PASSWORD`.

The `backend/`, `client/`, and `contracts/` directories are intentionally scaffolds;
application features have not been implemented.

The Android scaffold targets API 36 and Android 23+. Live camera scanning uses
`mobile_scanner` on Android. Linux uses image-import scanning; QR rendering and
image decoding are planned for the Rust `shared/qr` crate, exposed through the
core API.

## Agent work briefs

See [`agent-readmes/README.md`](agent-readmes/README.md) for parallel work rules,
file ownership, dependencies, and one brief for each implementation agent.

## CI builds and artifacts

GitHub Actions runs Flutter tests and builds for Android and Linux, plus backend
tests and a server build. Open the workflow run's **Artifacts** section to
download each platform build and its logs. Logs upload even when a test or build
fails; artifacts are retained for 14 days.
