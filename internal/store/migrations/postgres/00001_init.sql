-- +goose Up
CREATE TABLE meta (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE users (
	id TEXT PRIMARY KEY,
	email TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	password_hash TEXT NOT NULL,
	is_admin BOOLEAN NOT NULL DEFAULT FALSE,
	must_change_password BOOLEAN NOT NULL DEFAULT FALSE,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	disabled_at BIGINT
);

CREATE TABLE sessions (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	token_hash BYTEA NOT NULL UNIQUE,
	created_at BIGINT NOT NULL,
	last_seen_at BIGINT NOT NULL,
	expires_at BIGINT NOT NULL,
	ip TEXT NOT NULL DEFAULT '',
	user_agent TEXT NOT NULL DEFAULT ''
);
CREATE INDEX sessions_user_id ON sessions(user_id);

CREATE TABLE projects (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	slug TEXT NOT NULL UNIQUE,
	wrapped_dek BYTEA NOT NULL,
	dek_root_version BIGINT NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL
);

CREATE TABLE project_members (
	project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	role TEXT NOT NULL CHECK (role IN ('owner', 'editor', 'viewer')),
	created_at BIGINT NOT NULL,
	PRIMARY KEY (project_id, user_id)
);
CREATE INDEX project_members_user_id ON project_members(user_id);

CREATE TABLE environments (
	id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	slug TEXT NOT NULL,
	position BIGINT NOT NULL,
	created_at BIGINT NOT NULL,
	UNIQUE (project_id, slug)
);

CREATE TABLE secrets (
	id TEXT PRIMARY KEY,
	environment_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
	key TEXT NOT NULL,
	current_version BIGINT NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	deleted_at BIGINT
);
CREATE UNIQUE INDEX secrets_live_key ON secrets(environment_id, key) WHERE deleted_at IS NULL;

CREATE TABLE secret_versions (
	id TEXT PRIMARY KEY,
	secret_id TEXT NOT NULL REFERENCES secrets(id) ON DELETE CASCADE,
	version BIGINT NOT NULL,
	value_ct BYTEA NOT NULL,
	comment_ct BYTEA,
	actor_type TEXT NOT NULL,
	actor_id TEXT NOT NULL,
	created_at BIGINT NOT NULL,
	UNIQUE (secret_id, version)
);

CREATE TABLE service_tokens (
	id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	token_hash BYTEA NOT NULL UNIQUE,
	permission TEXT NOT NULL CHECK (permission IN ('read', 'read_write')),
	created_by TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	expires_at BIGINT,
	last_used_at BIGINT,
	revoked_at BIGINT,
	created_at BIGINT NOT NULL
);
CREATE INDEX service_tokens_project_id ON service_tokens(project_id);

CREATE TABLE service_token_environments (
	token_id TEXT NOT NULL REFERENCES service_tokens(id) ON DELETE CASCADE,
	environment_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
	PRIMARY KEY (token_id, environment_id)
);

CREATE TABLE personal_tokens (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	token_hash BYTEA NOT NULL UNIQUE,
	expires_at BIGINT,
	last_used_at BIGINT,
	revoked_at BIGINT,
	created_at BIGINT NOT NULL
);
CREATE INDEX personal_tokens_user_id ON personal_tokens(user_id);

CREATE TABLE audit_logs (
	id TEXT PRIMARY KEY,
	at BIGINT NOT NULL,
	actor_type TEXT NOT NULL,
	actor_id TEXT NOT NULL,
	actor_label TEXT NOT NULL,
	action TEXT NOT NULL,
	project_id TEXT,
	environment_id TEXT,
	target_type TEXT NOT NULL,
	target_id TEXT NOT NULL,
	metadata TEXT NOT NULL DEFAULT '{}',
	ip TEXT NOT NULL DEFAULT '',
	user_agent TEXT NOT NULL DEFAULT ''
);
CREATE INDEX audit_logs_at ON audit_logs(at DESC, id DESC);
CREATE INDEX audit_logs_project_at ON audit_logs(project_id, at DESC, id DESC);

-- +goose Down
DROP TABLE audit_logs;
DROP TABLE personal_tokens;
DROP TABLE service_token_environments;
DROP TABLE service_tokens;
DROP TABLE secret_versions;
DROP TABLE secrets;
DROP TABLE environments;
DROP TABLE project_members;
DROP TABLE projects;
DROP TABLE sessions;
DROP TABLE users;
DROP TABLE meta;
