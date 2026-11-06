# Architecture

Envily is one Go module with the SvelteKit UI in `web/`. The UI is built to
static files and embedded in the binary, so one binary is the server, the UI,
and the CLI.

## Packages

```
cmd/envily            entry point: the cobra root command
internal/cli          CLI commands (client commands use the HTTP API; admin commands open the database)
internal/config       environment variables and flags → validated Config
internal/server       wiring: config → database → keyring → service → HTTP; key check and rotation
internal/httpapi      HTTP handlers, middleware, JSON DTOs, embedded SPA handler
internal/service      business logic: auth, users, projects, secrets, tokens, audit, maintenance
internal/authz        principals, roles, and the permission matrix (Can)
internal/crypto       XChaCha20-Poly1305 sealing, keyring and DEK wrapping, tokens, argon2id
internal/store        Store interface and domain models
  sqlite/, postgres/  implementations on top of sqlc-generated queries
  storetest/          one test suite run against both dialects
  migrations/         goose migrations per dialect, embedded
internal/dotenv       .env parsing and formatting
internal/version      build information (set with -ldflags)
sql/{sqlite,postgres}/queries   sqlc query files, kept identical across dialects
web/                  SvelteKit SPA (Svelte 5, Tailwind CSS 4, Bits UI); web/embed.go embeds web/build
```

Dependencies point one way: `httpapi → service → store + crypto + authz`.

- Handlers never write SQL and never call `crypto` for secrets.
- `service` is the only layer that encrypts or decrypts secrets.
- `store` knows nothing about encryption; it stores ciphertext bytes.
- Every action that changes data or reads secret values writes an audit
  entry. Changes write it in the same transaction as the change; reads write
  it before the value is returned, and fail if it cannot be written.

## A request

```
client ─► recover ─► request ID ─► security headers ─► access log ─► body limit
       ─► authentication (cookie or bearer → Principal) ─► CSRF (cookie requests)
       ─► rate limit ─► handler ─► service (authz check, transaction, crypto) ─► store
```

Reading a secret: the service resolves the project and environment (404 when
the principal cannot see them), checks the permission, unwraps the project's
DEK with the master key, opens the value with associated data
`secret:<id>:<version>`, zeroes the DEK, writes `secret.read` to the audit log,
and returns the value.

Saving from the UI: the secret table keeps edits as a local draft and sends
one `POST …/secrets:batch` with `expectedVersion` per changed key. The service
applies deletes, renames, and upserts in one transaction and returns `409`
with the conflicting keys if any secret changed meanwhile, so nothing is
partially saved.

## Storage

- IDs are UUIDv7 strings created in Go; times are unix milliseconds; JSON is
  stored as text. The schema is identical in SQLite and PostgreSQL apart
  from column types.
- The repository code is written once in `internal/store/sqlite/repos.go`
  against the sqlc API, which is the same for both dialects. `make generate`
  runs sqlc and copies that file to `internal/store/postgres/repos_gen.go`.
  Edit only the SQLite copy.
- SQLite uses WAL, one writer connection, and a separate read pool.
  PostgreSQL uses pgx through `database/sql` with up to 10 connections.
- Each secret write creates a new row in `secret_versions` (at most 100 kept
  per secret). Deleting a secret is a soft delete; the key can be reused.

## Background work

A daily maintenance loop deletes expired sessions and audit entries older
than `ENVILY_AUDIT_RETENTION_DAYS`. There are no queues, caches, or other
services.

## Web UI

- SvelteKit with `adapter-static` in SPA mode (`ssr = false`, fallback
  `index.html`). The Go SPA handler serves files as they are, gives
  `/_app/immutable/*` a one-year immutable cache, and answers other non-API
  paths with `index.html` (no cache).
- `web/src/lib/api`: a typed fetch client (same-origin cookie, `X-Envily-Client: web`,
  errors become `ApiError`; `401` redirects to `/login`).
- `web/src/lib/components/ui`: thin Tailwind wrappers over Bits UI primitives.
- `web/src/lib/i18n`: every UI string, ready for translation.
- `web/src/lib/secrets/draft.ts`: the draft model behind the secret table
  (unit tested), separate from its component.
- In development, Vite serves the UI and proxies `/api` and `/healthz` to the
  Go server (`make dev`).

## Tests

- `make test`: Go unit and API tests on SQLite, and vitest.
- `make test-pg`: the shared store suite and API tests on PostgreSQL (testcontainers).
- `make e2e`: Playwright against the real binary with a throwaway database.
