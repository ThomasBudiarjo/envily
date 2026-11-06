-- name: InsertAuditLog :exec
INSERT INTO audit_logs (id, at, actor_type, actor_id, actor_label, action, project_id, environment_id,
	target_type, target_id, metadata, ip, user_agent)
VALUES (sqlc.arg(id), sqlc.arg(at), sqlc.arg(actor_type), sqlc.arg(actor_id), sqlc.arg(actor_label), sqlc.arg(action),
	sqlc.narg(project_id), sqlc.narg(environment_id), sqlc.arg(target_type), sqlc.arg(target_id),
	sqlc.arg(metadata), sqlc.arg(ip), sqlc.arg(user_agent));

-- name: ListAuditLogs :many
SELECT * FROM audit_logs
WHERE (CAST(sqlc.arg(project_id) AS TEXT) = '' OR project_id = sqlc.arg(project_id))
	AND (CAST(sqlc.arg(action) AS TEXT) = '' OR action = sqlc.arg(action))
	AND (CAST(sqlc.arg(actor_id) AS TEXT) = '' OR actor_id = sqlc.arg(actor_id))
	AND at >= sqlc.arg(from_at) AND at <= sqlc.arg(to_at)
	AND (at < sqlc.arg(cursor_at) OR (at = sqlc.arg(cursor_at) AND id < sqlc.arg(cursor_id)))
ORDER BY at DESC, id DESC
LIMIT CAST(sqlc.arg(row_limit) AS BIGINT);

-- name: DeleteAuditLogsBefore :execrows
DELETE FROM audit_logs WHERE at < sqlc.arg(before);
