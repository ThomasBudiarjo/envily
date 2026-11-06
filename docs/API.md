# Envily HTTP API (v1)

The API is stable as of Phase 5. The web UI and the CLI use the same endpoints.

## Conventions

- Base path `/api/v1`. Requests and responses are JSON with camelCase fields;
  times are ISO 8601 UTC strings.
- Authentication: the `envily_session` cookie (web UI) or
  `Authorization: Bearer <token>` with a personal token (`envily_pat_…`) or a
  service token (`envily_svc_…`).
- Cookie requests that change data must send `X-Envily-Client: web` and an
  `Origin` (or `Referer`) equal to `ENVILY_PUBLIC_URL`. Bearer requests skip
  this check.
- Request bodies are limited to 1 MiB (5 MiB for `secrets:import`). Unknown
  JSON fields are rejected.
- Errors: `{"error": {"code": "…", "message": "…", "details": {…}}}`. Codes:
  `bad_request`, `validation_failed` (details per field), `unauthorized`,
  `invalid_credentials`, `forbidden`, `not_found`, `conflict`, `last_owner`,
  `rate_limited` (with `Retry-After`), `password_change_required`,
  `decryption_failed`, `internal`.
- Projects you are not a member of answer `404`, exactly like projects that
  do not exist. Environments outside a service token's scope also answer `404`.
- Lists return `{"items": […]}`. The audit log is paginated:
  `?limit=50&cursor=…` (max 200) returns `{"items": […], "nextCursor": "…" | null}`.
- Every response carries `X-Request-Id` (taken from the request when valid).
- `{p}` is a project slug, `{env}` an environment slug, `{key}` a secret key.

## Health

| Method | Path | Access | Notes |
|---|---|---|---|
| GET | `/healthz` | public | 200 while the process runs |
| GET | `/readyz` | public | 200 when the database answers and the master key matches |

## Setup and authentication

| Method | Path | Access | Body → response |
|---|---|---|---|
| GET | `/setup/status` | public | → `{needsSetup}` |
| POST | `/setup` | public, once | `{email, name, password, setupToken?}` → `201 {user}` + session cookie; `409` after the first user exists |
| POST | `/auth/login` | public | `{email, password}` → `{user}` + session cookie |
| POST | `/auth/cli-login` | public | `{email, password, tokenName?}` → `201 {token, expiresAt}` (personal token, 90 days) |
| POST | `/auth/logout` | user | → `204`, deletes the session |
| GET | `/auth/me` | user | → `{user, isAdmin, mustChangePassword}` |
| POST | `/auth/password` | user | `{currentPassword, newPassword}` → `204`; signs out other sessions |
| GET | `/me/tokens` | user | → `{items: [personalToken]}` |
| POST | `/me/tokens` | user | `{name, expiresAt?}` → `201 {token, info}` |
| DELETE | `/me/tokens/{id}` | user | → `204`; `id` may be `current` to revoke the calling personal token |

Passwords are 12–256 characters. Logins are limited to 5 per minute per IP
and 10 per 15 minutes per email; other calls to 20 per second (burst 40)
per principal. A user with `mustChangePassword` may call only `auth/me`,
`auth/password`, and `auth/logout`.

## Users and instance audit (admins)

| Method | Path | Body → response |
|---|---|---|
| GET | `/users` | → `{items: [user]}` |
| POST | `/users` | `{email, name, isAdmin?}` → `201 {user, temporaryPassword}` |
| PATCH | `/users/{id}` | `{name?, isAdmin?, disabled?, resetPassword?}` → `{user, temporaryPassword?}`; disabling or resetting revokes all sessions and personal tokens |
| GET | `/audit` | `?action=&actor=&from=&to=&limit=&cursor=` → page of audit entries |

## Projects, members, environments

| Method | Path | Access | Body → response |
|---|---|---|---|
| GET | `/projects` | user / token | → `{items: [project]}` (members: their projects; admins: all; service token: its project) |
| POST | `/projects` | user | `{name, slug?}` → `201 project` with `development`, `staging`, `production` |
| GET | `/projects/{p}` | member | → `project` (`role` is `null` for admins who are not members) |
| PATCH | `/projects/{p}` | owner | `{name?, slug?}` → `project` |
| DELETE | `/projects/{p}` | owner | `{confirm: "<slug>"}` → `204` |
| GET | `/projects/{p}/members` | member | → `{items: [member]}` |
| POST | `/projects/{p}/members` | owner (admins: themselves) | `{userId or email, role}` → `201 member` |
| PATCH | `/projects/{p}/members/{userId}` | owner | `{role}`; `409 last_owner` for the last owner |
| DELETE | `/projects/{p}/members/{userId}` | owner, or the member | → `204`; `409 last_owner` for the last owner |
| GET | `/projects/{p}/environments` | member | → `{items: [environment]}` |
| POST | `/projects/{p}/environments` | owner | `{name, slug?}` → `201 environment` |
| PATCH | `/projects/{p}/environments/{env}` | owner | `{name?, slug?, position?}` |
| DELETE | `/projects/{p}/environments/{env}` | owner | `{confirm: "<env slug>"}` → `204` |
| GET | `/projects/{p}/compare` | member | `?envs=development,production` → `{environments, keys: {KEY: {env: {exists, version?, updatedAt?}}}}`, no values |
| GET | `/projects/{p}/audit` | editor, owner | as `/audit`, limited to the project |

Environment slugs `compare`, `settings`, and `audit` are reserved (they are UI
routes next to `/p/{project}/{env}`).

Roles: owner, editor, viewer. See the permission matrix in `internal/authz`.

## Secrets

Prefix: `/projects/{p}/environments/{env}/secrets`.

| Method | Path | Access | Body → response |
|---|---|---|---|
| GET | `` | read | → `{items: [{key, version, createdAt, updatedAt, updatedBy, hasComment}]}`; `?values=true` adds `value` and `comment` (audited as `secrets.read_all`) |
| GET | `/{key}` | read | → `{key, value, comment, version, …}` (audited as `secret.read`) |
| PUT | `/{key}` | write | `{value, comment?, expectedVersion?}` → `201` created / `200` updated; omitted `comment` keeps the current one; a stale `expectedVersion` gives `409` |
| PATCH | `/{key}` | write | `{newKey}` → `204`; versions are kept |
| DELETE | `/{key}` | write | → `204` (soft delete; the key can be reused) |
| POST | `:batch` | write | `{upserts: [{key, value, comment?, expectedVersion?}], deletes: [key], renames: [{from, to}]}` → `{created, updated, deleted, renamed}`; atomic, at most 1000 changes |
| POST | `:import` | write | body is `.env` text or a JSON object of strings; `?mode=merge` (default) or `overwrite` (deletes keys not in the input) → `{created, updated, unchanged, deleted}` |
| GET | `:export` | read | `?format=dotenv` (default), `json`, or `yaml` → file download (audited as `secrets.export`) |
| GET | `/{key}/versions` | member (viewers without values) | → `{items: [{version, createdAt, actor}]}`; `?values=true` for editors and owners |
| POST | `/{key}/rollback` | editor, owner | `{version}` → `{version}` (a new version with the old value) |

Keys match `^[A-Za-z_][A-Za-z0-9_]{0,254}$`. Values are at most 64 KiB,
comments 1 KiB. The latest 100 versions of each secret are kept.

## Service tokens

| Method | Path | Access | Body → response |
|---|---|---|---|
| GET | `/projects/{p}/service-tokens` | owner | → `{items: [{id, name, permission, environments, createdAt, expiresAt, lastUsedAt, revokedAt}]}` |
| POST | `/projects/{p}/service-tokens` | owner | `{name, environments: [slug], permission: "read" or "read_write", expiresAt?}` → `201 {token, info}`; the token is shown only once |
| DELETE | `/projects/{p}/service-tokens/{id}` | owner | → `204`, effective immediately |
| GET | `/token/info` | service token | → `{name, project, environments, permission, expiresAt}` |

A `read` token may list and read secrets in its environments; `read_write`
may also write them. Neither may see history, members, tokens, or audit logs.
