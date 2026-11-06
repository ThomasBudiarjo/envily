-- name: CreateSecret :exec
INSERT INTO secrets (id, environment_id, key, current_version, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(environment_id), sqlc.arg(key), sqlc.arg(current_version),
	sqlc.arg(created_at), sqlc.arg(updated_at));

-- name: GetLiveSecret :one
SELECT * FROM secrets
WHERE environment_id = sqlc.arg(environment_id) AND key = sqlc.arg(key) AND deleted_at IS NULL;

-- name: UpdateSecret :execrows
UPDATE secrets SET key = sqlc.arg(key), current_version = sqlc.arg(current_version),
	updated_at = sqlc.arg(updated_at), deleted_at = sqlc.narg(deleted_at)
WHERE id = sqlc.arg(id);

-- name: ListLiveSecrets :many
SELECT s.id, s.environment_id, s.key, s.current_version, s.created_at, s.updated_at,
	v.value_ct, v.comment_ct, v.actor_type, v.actor_id
FROM secrets s
JOIN secret_versions v ON v.secret_id = s.id AND v.version = s.current_version
WHERE s.environment_id = sqlc.arg(environment_id) AND s.deleted_at IS NULL
ORDER BY s.key;

-- name: ListProjectSecretKeys :many
SELECT s.environment_id, s.key, s.current_version, s.updated_at
FROM secrets s JOIN environments e ON e.id = s.environment_id
WHERE e.project_id = sqlc.arg(project_id) AND s.deleted_at IS NULL
ORDER BY s.key;

-- name: CreateSecretVersion :exec
INSERT INTO secret_versions (id, secret_id, version, value_ct, comment_ct, actor_type, actor_id, created_at)
VALUES (sqlc.arg(id), sqlc.arg(secret_id), sqlc.arg(version), sqlc.arg(value_ct), sqlc.arg(comment_ct),
	sqlc.arg(actor_type), sqlc.arg(actor_id), sqlc.arg(created_at));

-- name: GetSecretVersion :one
SELECT * FROM secret_versions WHERE secret_id = sqlc.arg(secret_id) AND version = sqlc.arg(version);

-- name: ListSecretVersions :many
SELECT * FROM secret_versions WHERE secret_id = sqlc.arg(secret_id) ORDER BY version DESC;

-- name: PruneSecretVersions :exec
DELETE FROM secret_versions WHERE secret_id = sqlc.arg(secret_id) AND version < sqlc.arg(min_version);
