# Contributing to Caddystat

Thanks for helping improve Caddystat! Please follow these guidelines to keep things smooth.

## Getting started

- Fork and clone the repo.
- Install Go (1.22+) and Node (for the web assets).
- Run the API locally: `go run ./cmd/caddystat`.
- Build the frontend (from `web/`): `npm install && npm run build`.

## Development workflow

- Keep changes small and focused; open an issue before large refactors.
- Add tests when you add behavior or fix a bug. Go tests live alongside code; frontend tests belong in `web/`.
- Run `go test ./...` before sending a PR. For UI changes, include screenshots or short notes.
- Follow Go formatting (`go fmt`) and keep configs in `Caddyfile`/`docker-compose.yml` consistent with examples in `README.md`.
- Document new env vars or config flags in `README.md`.

## Pull requests

- Describe the problem, the approach, and any trade-offs.
- Note any breaking changes.
- Ensure CI/tests pass; link related issues if they exist.

## Code of Conduct

By participating, you agree to follow the [Code of Conduct](CODE_OF_CONDUCT.md).
