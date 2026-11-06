import { request } from './client';
import type { User } from './types';

export const listUsers = () => request<{ items: User[] }>('/users').then((r) => r.items);

export const createUser = (body: { email: string; name: string; isAdmin: boolean }) =>
	request<{ user: User; temporaryPassword: string }>('/users', { method: 'POST', body });

export const updateUser = (
	id: string,
	body: { name?: string; isAdmin?: boolean; disabled?: boolean; resetPassword?: boolean }
) =>
	request<{ user: User; temporaryPassword?: string }>(`/users/${encodeURIComponent(id)}`, {
		method: 'PATCH',
		body
	});
