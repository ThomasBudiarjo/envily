-- name: CreateServiceToken :exec
INSERT INTO service_tokens (id, project_id, name, token_hash, permission, created_by, expires_at, created_at)
VALUES (sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(name), sqlc.arg(token_hash), sqlc.arg(permission),
	sqlc.arg(created_by), sqlc.narg(expires_at), sqlc.arg(created_at));

-- name: AddServiceTokenEnvironment :exec
INSERT INTO service_token_environments (token_id, environment_id) VALUES (sqlc.arg(token_id), sqlc.arg(environment_id));

-- name: GetServiceTokenByHash :one
SELECT * FROM service_tokens WHERE token_hash = sqlc.arg(token_hash);

-- name: GetServiceToken :one
SELECT * FROM service_tokens WHERE id = sqlc.arg(id);

-- name: ListServiceTokens :many
SELECT * FROM service_tokens WHERE project_id = sqlc.arg(project_id) ORDER BY created_at DESC, id DESC;

-- name: ListServiceTokenEnvironments :many
SELECT te.token_id, te.environment_id
FROM service_token_environments te JOIN service_tokens t ON t.id = te.token_id
WHERE t.project_id = sqlc.arg(project_id);

-- name: RevokeServiceToken :execrows
UPDATE service_tokens SET revoked_at = sqlc.arg(revoked_at) WHERE id = sqlc.arg(id) AND revoked_at IS NULL;

-- name: TouchServiceToken :exec
UPDATE service_tokens SET last_used_at = sqlc.arg(last_used_at) WHERE id = sqlc.arg(id);

-- name: CreatePersonalToken :exec
INSERT INTO personal_tokens (id, user_id, name, token_hash, expires_at, created_at)
VALUES (sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(name), sqlc.arg(token_hash), sqlc.narg(expires_at), sqlc.arg(created_at));

-- name: GetPersonalTokenByHash :one
SELECT * FROM personal_tokens WHERE token_hash = sqlc.arg(token_hash);

-- name: ListPersonalTokens :many
SELECT * FROM personal_tokens WHERE user_id = sqlc.arg(user_id) ORDER BY created_at DESC, id DESC;

-- name: RevokePersonalToken :execrows
UPDATE personal_tokens SET revoked_at = sqlc.arg(revoked_at)
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id) AND revoked_at IS NULL;

-- name: RevokeUserPersonalTokens :exec
UPDATE personal_tokens SET revoked_at = sqlc.arg(revoked_at) WHERE user_id = sqlc.arg(user_id) AND revoked_at IS NULL;

-- name: TouchPersonalToken :exec
UPDATE personal_tokens SET last_used_at = sqlc.arg(last_used_at) WHERE id = sqlc.arg(id);
