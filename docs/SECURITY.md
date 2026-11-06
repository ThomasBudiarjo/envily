# Security

This document describes how Envily protects secrets, what it does not
protect against, and the hardening built into the server and UI.

## Reporting a vulnerability

Please report security problems privately to the maintainer instead of
opening a public issue.

## Cryptography

Envily uses two-level envelope encryption. All encryption and decryption
goes through `internal/crypto`; no other package calls a cipher directly.

```
master key (32 bytes, process memory only, from ENVILY_MASTER_KEY[_FILE])
  └─ wraps one data encryption key (DEK) per project   → projects.wrapped_dek
       └─ encrypts secret values and comments          → secret_versions.value_ct / comment_ct
```

- **AEAD:** XChaCha20-Poly1305 (`golang.org/x/crypto/chacha20poly1305`) with
  a fresh random 24-byte nonce from `crypto/rand` for every encryption.
- **Stored format:** `0x01` (format version) ‖ nonce (24 bytes) ‖ ciphertext and tag.
- **Keys:** 32 random bytes from `crypto/rand`.
- **Associated data binds each ciphertext to its row**, so ciphertext copied
  to another row, version, or project fails to decrypt:
  - DEK: `dek:<project id>:<root key version>`
  - value: `secret:<secret id>:<version>`
  - comment: `comment:<secret id>:<version>`
- **Key check:** the database stores `key_check`, a sealed known string. At
  every start the server opens it and refuses to run ("master key does not
  match this database") when the key is wrong.
- **DEKs** are unwrapped per request, never cached, and zeroed after use (best effort in Go).
- **Rotation:** `envily rotate-master-key` re-wraps every DEK with a new key
  under the next root key version and rewrites `key_check` in one transaction.
- Secret **key names are stored in plaintext** so they can be searched and
  compared across environments. Values and comments are encrypted.

### Tokens and passwords

- Session cookies, personal tokens (`envily_pat_…`), and service tokens
  (`envily_svc_…`) are 32 random bytes, base64url-encoded. Only their
  SHA-256 hash is stored; the high entropy makes a slow hash unnecessary.
  The prefixes let secret scanners recognise leaked tokens.
- Service and personal tokens are shown once, at creation.
- Passwords are hashed with argon2id (m = 19 MiB, t = 2, p = 1, 16-byte salt,
  PHC string). At most 4 hashes run at once to bound memory.
- Failed logins answer `invalid_credentials` whether or not the email
  exists, and run a dummy hash so the response time is the same.

## Threat model

**Protected:**

- A leaked database file or backup without the master key: no value or
  comment can be decrypted, and tokens cannot be recovered from their hashes.
- Ciphertext moved between rows, versions, or projects: decryption fails.
- Leaked token hashes: they cannot be used to authenticate.

**Not protected:**

- An attacker with root on the host running the server: the master key is
  in the process memory and in its environment or key file.
- A legitimate user abusing their access: every read of secret values and
  every change is written to the audit log, which is the mitigation.

**Losing the master key means losing every secret permanently.** Keep it
separate from database backups (see [SELF_HOSTING.md](SELF_HOSTING.md)).

## Authentication and authorization

- Three kinds of callers become one principal: web sessions (cookie),
  personal tokens (CLI, all permissions of the user), and service tokens
  (one project, chosen environments, `read` or `read_write`).
- Every handler goes through the service layer, which checks permissions
  with `authz` (see the matrix in `internal/authz`).
- Projects the caller cannot see answer `404`, exactly like missing ones.
  Environments outside a service token's scope also answer `404`.
- A project always keeps at least one owner (`last_owner`).
- Users with a temporary password may only change it or sign out.
- Disabling a user or resetting their password revokes their sessions and
  personal tokens. Changing a password signs out the user's other sessions.

### Sessions and CSRF

- Cookie `envily_session`: `HttpOnly`, `SameSite=Strict`, `Path=/`, and
  `Secure` when `ENVILY_PUBLIC_URL` is `https`.
- Idle timeout 12 hours, absolute lifetime 7 days (configurable).
- Requests that change data with a cookie must carry an `Origin` (or
  `Referer`) equal to `ENVILY_PUBLIC_URL` and the header `X-Envily-Client: web`.
  Bearer-token requests carry no ambient credentials and skip this check.

### Rate limits

- Sign-in: 5 attempts per minute per IP and 10 per 15 minutes per email.
- Everything else: 20 requests per second per principal (burst 40).
- Exceeding a limit answers `429` with `Retry-After`. Client IPs come from
  `X-Forwarded-For` only for requests from `ENVILY_TRUSTED_PROXIES`.

## HTTP hardening

- Response headers: a strict `Content-Security-Policy` (`default-src 'self'`,
  scripts only from `'self'` plus the hashes of the inline bootstrap script,
  `frame-ancestors 'none'`, `connect-src 'self'`), `X-Content-Type-Options: nosniff`,
  `Referrer-Policy: no-referrer`, `X-Frame-Options: DENY`, and a restrictive
  `Permissions-Policy`.
- The UI loads nothing from other origins: fonts are self-hosted and assets
  are never inlined as `data:` URLs.
- Request bodies are limited to 1 MiB (5 MiB for imports); unknown JSON
  fields are rejected; server timeouts bound slow clients.
- Panics are recovered and logged without request bodies.

## What never leaves the server in clear

- Secret values, comments, the master key, passwords, and tokens are never
  written to logs, error messages, or audit metadata. Audit entries record
  only key names and version numbers. Parse errors name lines and keys, never values.
- Access logs omit query strings.
- The UI keeps revealed values in memory only (never in localStorage,
  sessionStorage, URLs, or the console), hides them again after 60 seconds or
  on navigation, and forgets them unless they are part of an unsaved edit.
- The CLI writes credentials (`~/.config/envily/credentials.json`) and
  exported files with mode 0600. `envily run` passes secrets to the child
  process only and never writes them to disk.

## Deployment hardening

- The container runs as a non-root user (65532) on a distroless base with no
  shell; the SQLite file is created with mode 0600.
- Provide the master key as a file (`ENVILY_MASTER_KEY_FILE`, e.g. a Docker
  secret) rather than an environment variable.
- Set `ENVILY_SETUP_TOKEN` when the instance is reachable before setup.
- Always serve Envily over HTTPS in production.
