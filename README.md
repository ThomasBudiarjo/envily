# Envily

A small, self-hosted secrets manager. One Go binary contains the server, the
web UI, and the CLI. It stores secrets per project and environment in SQLite
(or PostgreSQL), encrypted at rest with envelope encryption.

- Projects with `development`, `staging`, and `production` environments (add your own).
- Key/value secrets with comments, version history, rollback, and soft delete.
- Web UI: secret table with drafts and atomic save, reveal on demand, import
  and export (`.env`, JSON, YAML), environment comparison, audit log.
- Roles per project (owner, editor, viewer) and instance admins.
- Service tokens for CI/CD scoped to environments, read or read/write.
- CLI: `envily run -- <command>` injects secrets as environment variables.
- One container, one volume: about 22 MB image, ~10 MB RAM idle.

## Quick start (Docker)

```sh
docker run --rm ghcr.io/thomasbudiarjo/envily keygen > master.key
chmod 600 master.key
docker run -d --name envily -p 8080:8080 \
  -v envily-data:/data \
  -v "$PWD/master.key:/run/secrets/master.key:ro" \
  -e ENVILY_MASTER_KEY_FILE=/run/secrets/master.key \
  -e ENVILY_PUBLIC_URL=http://localhost:8080 \
  ghcr.io/thomasbudiarjo/envily
```

Open http://localhost:8080 and create the first admin. For production (TLS,
reverse proxy, PostgreSQL, backups), read [docs/SELF_HOSTING.md](docs/SELF_HOSTING.md).

**Keep `master.key` safe and separate from your database backups.** Without
it, no secret can ever be decrypted.

## CLI

```sh
envily login --url https://secrets.example.com   # stores a personal token
envily link                                      # pick project + environment, writes .envily.json
envily secrets list
envily secrets set DATABASE_URL=postgres://… API_KEY=-   # "-" reads the value from stdin
envily run -- npm start                          # run with secrets as env vars
envily export --format dotenv --out .env.local   # written with mode 0600
```

In CI, use a service token instead of logging in:

```sh
ENVILY_URL=https://secrets.example.com ENVILY_TOKEN=envily_svc_… envily run -- ./deploy.sh
```

Context comes from flags (`--url`, `--project`, `--env`, `--token`), then the
`ENVILY_URL`, `ENVILY_PROJECT`, `ENVILY_ENV`, `ENVILY_TOKEN` variables, then the
nearest `.envily.json`, then the stored login. Exit codes: 0 success,
1 error, 2 usage, 3 not authenticated, 4 forbidden, 5 not found.

Server-side commands: `server`, `migrate`, `keygen`, `rotate-master-key`,
`backup`, `admin create-user`, `admin reset-password`, `healthcheck`,
`version`. Run `envily <command> --help` for details.

## Documentation

- [Self-hosting](docs/SELF_HOSTING.md): configuration, deployment, backups, key rotation, upgrades.
- [Security](docs/SECURITY.md): cryptography, threat model, and hardening.
- [Architecture](docs/ARCHITECTURE.md): packages and how a request flows.
- [HTTP API](docs/API.md): the v1 API used by the UI and the CLI.
- [Decisions](docs/DECISIONS.md): choices made where the plan was silent or conflicted.

## Development

Requirements: Go (latest stable), Node.js LTS, pnpm, and `sqlc` and
`golangci-lint` for code generation and linting.

```sh
make dev        # Go server on :8080 and Vite on :5173 (DEV_WEB_PORT=… to change)
make test       # Go tests (SQLite) and vitest
make test-pg    # store and API tests against PostgreSQL (needs Docker)
make e2e        # build, start a throwaway server, run Playwright
make lint       # golangci-lint, svelte-check, eslint, prettier
make build      # bin/envily with the UI embedded
make docker     # container image (DOCKER=podman to use Podman)
make generate   # sqlc and the PostgreSQL copy of the store
```
