# Gate 5 production acceptance checks

The production-operations branch is accepted only when GitHub Actions passes all of these checks together on the same commit:

- Go module files remain tidy and locked.
- All Go sources are formatted and pass `go vet`.
- Race-enabled unit and PostgreSQL integration tests pass.
- qh8z, healthcheck, and load-test binaries build.
- Development and production Compose files validate.
- The production qh8z container builds.
- The Caddy production configuration adapts/validates with the pinned official image.
- Prometheus configuration and alert rules pass `promtool`.
- Alertmanager configuration passes `amtool`.
- Deployment, backup, restore, and failure-test shell scripts pass syntax validation.
- The backup image builds.
- A production-shaped qh8z/PostgreSQL pair becomes ready.
- A seeded redirect survives a concurrent load test and records visits.
- An encrypted restic backup is created.
- That backup restores into a fresh PostgreSQL database and contains the seeded link.
- Readiness fails while PostgreSQL is deliberately stopped and recovers after PostgreSQL and qh8z restart.

A script or manifest merely existing in the repository is not enough to mark the corresponding Issue #3 Gate 5 item complete. The same candidate commit must pass the automated rehearsal above, followed by the documented launch-host smoke checks before public launch.
