import { request } from './client';
import type { ServiceToken, TokenPermission } from './types';

const base = (project: string) => `/projects/${encodeURIComponent(project)}/service-tokens`;

export const listServiceTokens = (project: string) =>
	request<{ items: ServiceToken[] }>(base(project)).then((r) => r.items);

export const createServiceToken = (
	project: string,
	body: { name: string; environments: string[]; permission: TokenPermission; expiresAt?: string }
) => request<{ token: string; info: ServiceToken }>(base(project), { method: 'POST', body });

export const revokeServiceToken = (project: string, id: string) =>
	request<void>(`${base(project)}/${encodeURIComponent(id)}`, { method: 'DELETE' });
