-- name: GetMeta :one
SELECT value FROM meta WHERE key = sqlc.arg(key);

-- name: SetMeta :exec
INSERT INTO meta (key, value) VALUES (sqlc.arg(key), sqlc.arg(value))
ON CONFLICT (key) DO UPDATE SET value = excluded.value;

-- name: CreateUser :exec
INSERT INTO users (id, email, name, password_hash, is_admin, must_change_password, created_at, updated_at, disabled_at)
VALUES (sqlc.arg(id), sqlc.arg(email), sqlc.arg(name), sqlc.arg(password_hash), sqlc.arg(is_admin),
	sqlc.arg(must_change_password), sqlc.arg(created_at), sqlc.arg(updated_at), sqlc.narg(disabled_at));

-- name: GetUser :one
SELECT * FROM users WHERE id = sqlc.arg(id);

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = sqlc.arg(email);

-- name: ListUsers :many
SELECT * FROM users ORDER BY email;

-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: UpdateUser :execrows
UPDATE users SET name = sqlc.arg(name), password_hash = sqlc.arg(password_hash), is_admin = sqlc.arg(is_admin),
	must_change_password = sqlc.arg(must_change_password), updated_at = sqlc.arg(updated_at),
	disabled_at = sqlc.narg(disabled_at)
WHERE id = sqlc.arg(id);

-- name: CreateSession :exec
INSERT INTO sessions (id, user_id, token_hash, created_at, last_seen_at, expires_at, ip, user_agent)
VALUES (sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(token_hash), sqlc.arg(created_at), sqlc.arg(last_seen_at),
	sqlc.arg(expires_at), sqlc.arg(ip), sqlc.arg(user_agent));

-- name: GetSessionByHash :one
SELECT * FROM sessions WHERE token_hash = sqlc.arg(token_hash);

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = sqlc.arg(last_seen_at) WHERE id = sqlc.arg(id);

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = sqlc.arg(id);

-- name: DeleteUserSessions :exec
DELETE FROM sessions WHERE user_id = sqlc.arg(user_id) AND id <> sqlc.arg(except_id);

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= sqlc.arg(now);

-- name: InsertMeta :execrows
INSERT INTO meta (key, value) VALUES (sqlc.arg(key), sqlc.arg(value)) ON CONFLICT (key) DO NOTHING;
