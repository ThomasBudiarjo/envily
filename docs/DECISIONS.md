# Decisions

Choices made during implementation where the plan was silent, ambiguous, or
conflicted with the tools as they exist today. Newest last.

## SvelteKit 3 has no `svelte.config.js`

The plan lists `web/svelte.config.js`. SvelteKit 3 (current stable) no longer
reads that file: configuration is passed to the `sveltekit()` Vite plugin in
`web/vite.config.ts`, and the `$lib` alias is now `#lib` (a package.json
`imports` entry). The adapter, SPA fallback, and CSP settings live there.

## `go.mod` targets Go 1.26

The latest stable Go is 1.27, but `go.mod` declares `go 1.26.0` so the module
builds with any supported toolchain (including the one on the development
machine, which pins `GOTOOLCHAIN=local`). CI, Docker, and releases build with
the latest stable Go.

## `ENVILY_PUBLIC_URL` is always required

The plan says it is required "in production". The server cannot tell
production from development, and the Origin check and the cookie `Secure`
attribute both depend on it, so it is required everywhere. `make dev` sets it
to the Vite dev server URL.

## Placeholder for an unbuilt UI

`go:embed` needs at least one file in `web/build`. Instead of committing a
placeholder `index.html` (which every UI build would overwrite and leave as a
modified tracked file), the repository tracks `web/build/.gitkeep` and the Go
SPA handler serves a built-in placeholder page when `index.html` is missing.

## Access logs never include query strings

The plan asks to omit query strings for secret paths. The access log omits
them for every path, which is simpler and strictly safer.

## One store implementation for both dialects

sqlc generates a separate Go package per dialect, but with the queries kept
identical the two packages have the same Go API. The repository code is
therefore written once in `internal/store/sqlite/repos.go` and copied to
`internal/store/postgres/repos_gen.go` by `go generate` (only the import path
and package name change). CI fails when the copy is stale. Each dialect keeps
its own hand-written file for opening connections, migrations, and mapping
driver errors to `store.ErrNotFound` / `store.ErrConflict`.

## Store models use `time.Time`

Times are stored as unix milliseconds (BIGINT) in both dialects, as the plan
requires; the store converts them so the rest of the code works with UTC
`time.Time` values.

## PostgreSQL tests

`make test-pg` sets `ENVILY_TEST_POSTGRES=1`. The tests then use
`ENVILY_TEST_POSTGRES_DSN` when set, or start `postgres:18-alpine` with
testcontainers, creating a fresh database per test. Without Docker they skip,
except when `CI` is set. Podman works too:
`podman system service --time=0 unix:///tmp/podman.sock &` then
`DOCKER_HOST=unix:///tmp/podman.sock TESTCONTAINERS_RYUK_DISABLED=true make test-pg`.
