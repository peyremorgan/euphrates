# Devcontainer Compose IRC sidecar

Added a Docker Compose-based devcontainer topology and test infrastructure for realistic IRC development/testing.

## What changed

- Switched devcontainer definition to Compose mode with `app` and `irc` services.
- Added `.devcontainer/docker-compose.yml` with:
  - `app`: Go dev container mounted at `/workspace`
  - `irc`: Ergo server (`ghcr.io/ergochat/ergo:stable`)
- Added `.devcontainer/ergo/ircd.yaml` to configure a plaintext test listener on `:6667`.
- Added `remoteEnv`/service env defaults for `IRC_TEST_ADDR=irc:6667`.

## Testing updates

- Added in-process e2e test lane in `internal/irc/client_e2e_test.go`.
- Added reusable network scenario and mini IRC test server harness in `internal/irc/e2e_helpers_test.go`.
- Added live-server integration lane behind build tag in `internal/irc/client_integration_test.go`.
- Added `main_test.go` unit tests for channel parsing.
- Added Make targets:
  - `make test-e2e`
  - `make test-integration`

## Notes

- Integration tests are opt-in and intended for environments where a live IRC server is available (such as the devcontainer sidecar).
- The initial sidecar setup is plaintext-only to avoid self-signed TLS verification friction in test/dev workflows.
