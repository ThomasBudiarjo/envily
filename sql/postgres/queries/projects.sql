-- name: CreateProject :exec
INSERT INTO projects (id, name, slug, wrapped_dek, dek_root_version, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(name), sqlc.arg(slug), sqlc.arg(wrapped_dek), sqlc.arg(dek_root_version),
	sqlc.arg(created_at), sqlc.arg(updated_at));

-- name: GetProject :one
SELECT * FROM projects WHERE id = sqlc.arg(id);

-- name: GetProjectBySlug :one
SELECT * FROM projects WHERE slug = sqlc.arg(slug);

-- name: ListProjects :many
SELECT * FROM projects ORDER BY name, id;

-- name: ListProjectsForUser :many
SELECT p.* FROM projects p
JOIN project_members m ON m.project_id = p.id
WHERE m.user_id = sqlc.arg(user_id)
ORDER BY p.name, p.id;

-- name: UpdateProject :execrows
UPDATE projects SET name = sqlc.arg(name), slug = sqlc.arg(slug), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: UpdateProjectDEK :execrows
UPDATE projects SET wrapped_dek = sqlc.arg(wrapped_dek), dek_root_version = sqlc.arg(dek_root_version)
WHERE id = sqlc.arg(id);

-- name: DeleteProject :execrows
DELETE FROM projects WHERE id = sqlc.arg(id);

-- name: AddMember :exec
INSERT INTO project_members (project_id, user_id, role, created_at)
VALUES (sqlc.arg(project_id), sqlc.arg(user_id), sqlc.arg(role), sqlc.arg(created_at));

-- name: GetMember :one
SELECT * FROM project_members WHERE project_id = sqlc.arg(project_id) AND user_id = sqlc.arg(user_id);

-- name: ListMembers :many
SELECT m.project_id, m.user_id, m.role, m.created_at, u.email, u.name
FROM project_members m JOIN users u ON u.id = m.user_id
WHERE m.project_id = sqlc.arg(project_id)
ORDER BY u.email;

-- name: UpdateMemberRole :execrows
UPDATE project_members SET role = sqlc.arg(role)
WHERE project_id = sqlc.arg(project_id) AND user_id = sqlc.arg(user_id);

-- name: RemoveMember :execrows
DELETE FROM project_members WHERE project_id = sqlc.arg(project_id) AND user_id = sqlc.arg(user_id);

-- name: CountOwners :one
SELECT COUNT(*) FROM project_members WHERE project_id = sqlc.arg(project_id) AND role = 'owner';

-- name: CreateEnvironment :exec
INSERT INTO environments (id, project_id, name, slug, position, created_at)
VALUES (sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(name), sqlc.arg(slug), sqlc.arg(position), sqlc.arg(created_at));

-- name: GetEnvironment :one
SELECT * FROM environments WHERE id = sqlc.arg(id);

-- name: GetEnvironmentBySlug :one
SELECT * FROM environments WHERE project_id = sqlc.arg(project_id) AND slug = sqlc.arg(slug);

-- name: ListEnvironments :many
SELECT * FROM environments WHERE project_id = sqlc.arg(project_id) ORDER BY position, id;

-- name: UpdateEnvironment :execrows
UPDATE environments SET name = sqlc.arg(name), slug = sqlc.arg(slug), position = sqlc.arg(position)
WHERE id = sqlc.arg(id);

-- name: DeleteEnvironment :execrows
DELETE FROM environments WHERE id = sqlc.arg(id);

-- name: TouchProject :exec
UPDATE projects SET updated_at = sqlc.arg(updated_at) WHERE id = sqlc.arg(id);
