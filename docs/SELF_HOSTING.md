# Self-hosting Envily

Envily is one static binary (or a ~22 MB container image) with SQLite as the
default database. The standard deployment is one container, one volume
(`/data`), behind a reverse proxy that terminates TLS.

## 1. Create the master key

```sh
docker run --rm ghcr.io/thomasbudiarjo/envily keygen > master.key
chmod 600 master.key
```

The master key encrypts every project's data key. **If you lose it, every
secret is lost permanently.** Store a copy somewhere safe (a password manager
or an offline vault), and never in the same place as your database backups:
a backup together with its key is a full copy of your secrets.

Prefer `ENVILY_MASTER_KEY_FILE` (a Docker secret or a mounted file) over
`ENVILY_MASTER_KEY`: environment variables are visible to anyone who can
inspect the container.

## 2. Run it

### Docker Compose with Caddy (recommended)

`deploy/` contains ready-made files:

| File | Use |
|---|---|
| `docker-compose.yml` | Envily (SQLite) + Caddy with automatic HTTPS |
| `docker-compose.postgres.yml` | Envily + PostgreSQL + Caddy |
| `Caddyfile` | reverse proxy for `{$ENVILY_DOMAIN}` |

```sh
cd deploy
mkdir -p secrets && docker run --rm ghcr.io/thomasbudiarjo/envily keygen > secrets/master.key
chmod 600 secrets/master.key
export ENVILY_DOMAIN=secrets.example.com
export ENVILY_PUBLIC_URL=https://secrets.example.com
export ENVILY_SETUP_TOKEN=$(openssl rand -hex 16)   # optional, see below
docker compose up -d
```

Then open `https://secrets.example.com`, enter the setup token, and create the
first admin.

### Plain binary

```sh
envily keygen > /etc/envily/master.key && chmod 600 /etc/envily/master.key
ENVILY_PUBLIC_URL=https://secrets.example.com \
ENVILY_MASTER_KEY_FILE=/etc/envily/master.key \
ENVILY_DB_DSN=/var/lib/envily/envily.db \
envily server
```

Run it under a process supervisor (systemd) as an unprivileged user. The
database file is created with mode 0600.

### First admin and the setup token

Until the first user exists, anyone who reaches the server can create the
admin account. If the server is reachable from the internet before you set
it up, set `ENVILY_SETUP_TOKEN`; setup then requires that token. You can
also create the admin from the host without the UI:

```sh
envily admin create-user --email you@example.com --admin
```

## Configuration

All settings are environment variables. `envily server` also accepts flags
for the most common ones, which override the variables: `--addr`,
`--public-url`, `--db-driver`, `--db-dsn`, `--log-level`, `--log-format`.

| Variable | Default | Notes |
|---|---|---|
| `ENVILY_PUBLIC_URL` | (required) | The URL users open, e.g. `https://secrets.example.com`. Used for the CSRF Origin check and for the cookie `Secure` flag (set for `https`). |
| `ENVILY_MASTER_KEY_FILE` | | Path to a file with the base64 master key. |
| `ENVILY_MASTER_KEY` | | The base64 master key itself. Set exactly one of the two. |
| `ENVILY_ADDR` | `:8080` | Listen address. |
| `ENVILY_DB_DRIVER` | `sqlite` | `sqlite` or `postgres`. |
| `ENVILY_DB_DSN` | `/data/envily.db` | SQLite file path, or a `postgres://` URL. |
| `ENVILY_AUTO_MIGRATE` | `true` | Run migrations at start. Set `false` to run `envily migrate` yourself. |
| `ENVILY_SETUP_TOKEN` | | Token required to create the first admin. |
| `ENVILY_TRUSTED_PROXIES` | | Comma-separated IPs or CIDRs whose `X-Forwarded-For` is trusted for client IPs (rate limits, audit log). |
| `ENVILY_SESSION_IDLE_TIMEOUT` | `12h` | Web sessions end after this much inactivity. |
| `ENVILY_SESSION_MAX_AGE` | `168h` | Web sessions end after this long regardless of activity. |
| `ENVILY_AUDIT_RETENTION_DAYS` | `365` | Older audit entries are deleted daily. `0` keeps them forever. |
| `ENVILY_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error`. |
| `ENVILY_LOG_FORMAT` | `json` | `json` or `text`. |

The server refuses to start, with a message naming the problem, when the
configuration is invalid or the master key does not match the database.

## Reverse proxy

Envily speaks plain HTTP; put a TLS-terminating proxy in front of it and set
`ENVILY_PUBLIC_URL` to the `https://` URL. Set `ENVILY_TRUSTED_PROXIES` to the
proxy's address (or network), or every request appears to come from the
proxy and shares one rate limit. Do not trust a range that untrusted clients
can send from.

For nginx, forward `Host` and `X-Forwarded-For` and allow request bodies of at
least 5 MiB (imports):

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    client_max_body_size 6m;
}
```

## Health checks

- `GET /healthz`: 200 while the process runs (liveness).
- `GET /readyz`: 200 when the database answers and the master key matches (readiness).
- `envily healthcheck`: exits 0 when `/readyz` on `ENVILY_ADDR` answers 200.
  The image uses it as its Docker `HEALTHCHECK`, since it has no shell or curl.

## PostgreSQL

Set `ENVILY_DB_DRIVER=postgres` and `ENVILY_DB_DSN=postgres://user:pass@host/db`.
The pool uses at most 10 connections. Migrations run at start under an
advisory lock, so several instances can start at once.

## Backups

**SQLite:** `envily backup --out FILE` writes a consistent snapshot with
`VACUUM INTO` while the server runs:

```sh
docker exec envily envily backup --out /data/backup-$(date +%F).db
```

**PostgreSQL:** use `pg_dump`. `envily backup` refuses to run.

Backups contain only encrypted values and token hashes. Store them away from
the master key. To restore, stop the server, put the backup file in place of
the database (SQLite) or `pg_restore` it, and start the server with the same
master key that was current when the backup was taken.

## Rotating the master key

Rotation re-wraps each project's data key; secret values are not re-encrypted,
so it takes well under a second even with many secrets.

1. Create the new key: `envily keygen > new.key`.
2. Take a backup.
3. **Stop every server instance.** A running server would keep using the old key.
4. Run, with the current key configured as usual:
   ```sh
   envily rotate-master-key --new-key-file new.key
   ```
   It runs in one transaction: on any error nothing changes.
5. Replace the configured key with `new.key` and start the server.
6. Keep the old key until you have confirmed the new one works, then destroy
   it. Backups taken before the rotation still need the old key.

## Upgrades

Replace the binary or pull the new image and restart. Migrations run
automatically (unless `ENVILY_AUTO_MIGRATE=false`; then run `envily migrate`
first). Take a backup before upgrading.

## Admin tasks on the host

```sh
envily admin reset-password --email user@example.com   # prints a temporary password
envily admin create-user --email user@example.com [--admin]
```

These commands use the database directly and need the same `ENVILY_DB_*`
settings as the server, but not the master key.

## Notes for users

- Copied secret values stay in the system clipboard until something else is
  copied; browsers cannot clear it reliably.
- Revealed values in the web UI are hidden again after 60 seconds or when you
  leave the page.
