// Types mirror the JSON DTOs in internal/httpapi. Times are ISO 8601 strings.

export type Role = 'owner' | 'editor' | 'viewer';
export type TokenPermission = 'read' | 'read_write';
export type ExportFormat = 'dotenv' | 'json' | 'yaml';
export type ImportMode = 'merge' | 'overwrite';

export interface User {
	id: string;
	email: string;
	name: string;
	isAdmin: boolean;
	mustChangePassword: boolean;
	createdAt: string;
	updatedAt: string;
	disabledAt: string | null;
}

export interface Me {
	user: User;
	isAdmin: boolean;
	mustChangePassword: boolean;
}

export interface Environment {
	id: string;
	name: string;
	slug: string;
	position: number;
	createdAt: string;
}

export interface Project {
	id: string;
	name: string;
	slug: string;
	role: Role | null;
	createdAt: string;
	updatedAt: string;
	environments: Environment[];
}

export interface Member {
	userId: string;
	email: string;
	name: string;
	role: Role;
	createdAt: string;
}

export interface Actor {
	type: string;
	id: string;
	label: string;
}

export interface SecretItem {
	key: string;
	version: number;
	createdAt: string;
	updatedAt: string;
	updatedBy: Actor;
	hasComment: boolean;
	value?: string;
	comment?: string;
}

export interface Secret {
	key: string;
	value: string;
	comment: string;
	version: number;
	createdAt: string;
	updatedAt: string;
}

export interface SecretVersion {
	version: number;
	createdAt: string;
	actor: Actor;
	value?: string;
	comment?: string;
}

export interface BatchInput {
	upserts: { key: string; value: string; comment?: string; expectedVersion?: number }[];
	deletes: string[];
	renames: { from: string; to: string }[];
}

export interface BatchResult {
	created: number;
	updated: number;
	deleted: number;
	renamed: number;
}

export interface ImportResult {
	created: number;
	updated: number;
	unchanged: number;
	deleted: number;
}

export interface CompareCell {
	exists: boolean;
	version?: number;
	updatedAt?: string;
}

export interface Comparison {
	environments: Environment[];
	keys: Record<string, Record<string, CompareCell>>;
}

export interface PersonalToken {
	id: string;
	name: string;
	createdAt: string;
	expiresAt: string | null;
	lastUsedAt: string | null;
	revokedAt: string | null;
}

export interface ServiceToken extends PersonalToken {
	permission: TokenPermission;
	environments: string[];
}

export interface AuditEntry {
	id: string;
	at: string;
	actorType: string;
	actorId: string;
	actorLabel: string;
	action: string;
	projectId?: string;
	environmentId?: string;
	targetType: string;
	targetId: string;
	metadata: Record<string, unknown> | null;
	ip: string;
	userAgent: string;
}

export interface Page<T> {
	items: T[];
	nextCursor: string | null;
}

export interface AuditFilter {
	action?: string;
	actor?: string;
	from?: string;
	to?: string;
	cursor?: string;
	limit?: number;
}
