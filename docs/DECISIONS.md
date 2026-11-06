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

## Instance admins and projects they are not members of

The plan lets admins list every project and add themselves, but the
permission matrix gives a non-member admin no project permissions. Admins
therefore see such projects (in the list, the detail page, members, and
environments) with `role: null`, may add only themselves as a member, and
get `403` for everything else. Everyone else gets `404` for projects they
are not a member of, the same as for projects that do not exist.

## Adding members by email

`POST /projects/{p}/members` accepts `email` as an alternative to `userId`,
because project owners who are not admins cannot list instance users.

## Members may leave a project

Owners manage members; any member may also remove themselves. The last
owner can neither leave nor be demoted (`last_owner`).

## Secret writes

- `PUT …/secrets/{key}` without `comment` keeps the current comment; send
  `"comment": ""` to clear it.
- Import skips keys whose value is unchanged (no new version), so importing
  the same file twice is a no-op. The response reports `unchanged`.
- A batch applies deletes, then renames, then upserts, so a key can be
  deleted and another renamed onto it in one save.
- Parse and validation errors for imports name the line and the key but
  never quote values.
- Secret writes mark the project as updated (`updatedAt`), which the
  project list shows as "last changed".

## CLI details

- `DELETE /api/v1/me/tokens/current` revokes the calling personal token, so
  `envily logout` can revoke its own token without storing its ID.
- `envily run` drops `ENVILY_TOKEN` from the child's environment so the
  credential is not inherited by the command.
- The credentials file remembers the last server logged in to (`current`),
  used when neither `--url`, `ENVILY_URL`, nor `.envily.json` names one.
- Output is plain text without colour for now (the plan allows colour on a
  TTY only; none is needed yet).
- Argument and flag errors exit with 2, like other usage errors.

## Reserved environment slugs

The UI routes `/p/[project]/compare`, `/p/[project]/settings`, and
`/p/[project]/audit` sit next to `/p/[project]/[env]`, so an environment with
one of those slugs could never be opened in the web UI. The server rejects
`compare`, `settings`, and `audit` as environment slugs (on create and on
rename) with `validation_failed`.

## `rotate-master-key` requires a stopped server, on both databases

The plan allows running the rotation against PostgreSQL under an advisory
lock while servers run. A running server keeps the old master key in memory
and would wrap the DEKs of newly created projects with it, which the new key
cannot open. Rotation therefore always requires every server to be stopped;
the command re-wraps all DEKs and rewrites `key_check` in one transaction, so
a failure leaves the database unchanged.

## Two Dockerfiles

`Dockerfile` builds everything from source (UI, then Go) so `make docker` and
`docker build .` work without other tools. GoReleaser already builds the
binaries for every platform, so releases use `release.Dockerfile`, which only
copies the prebuilt binary. Both produce the same runtime image: distroless
`static:nonroot`, `/data` owned by the non-root user, and a `HEALTHCHECK`
that runs `envily healthcheck` because the image has no shell or curl.

## Node 26 in the image build

Node 26 is the current LTS line. It no longer bundles corepack, so the image
installs the pinned pnpm version with npm.
