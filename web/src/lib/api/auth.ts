import { request } from './client';
import type { Me, PersonalToken, User } from './types';

export const getSetupStatus = () =>
	request<{ needsSetup: boolean }>('/setup/status', { quiet: true });

export const setup = (body: {
	email: string;
	name: string;
	password: string;
	setupToken?: string;
}) => request<{ user: User }>('/setup', { method: 'POST', body, quiet: true });

export const login = (email: string, password: string) =>
	request<{ user: User }>('/auth/login', {
		method: 'POST',
		body: { email, password },
		quiet: true
	});

export const logout = () => request<void>('/auth/logout', { method: 'POST', quiet: true });

export const getMe = () => request<Me>('/auth/me', { quiet: true });

export const changePassword = (currentPassword: string, newPassword: string) =>
	request<void>('/auth/password', {
		method: 'POST',
		body: { currentPassword, newPassword },
		quiet: true
	});

export const listMyTokens = () =>
	request<{ items: PersonalToken[] }>('/me/tokens').then((r) => r.items);

export const createMyToken = (name: string, expiresAt?: string) =>
	request<{ token: string; info: PersonalToken }>('/me/tokens', {
		method: 'POST',
		body: { name, expiresAt }
	});

export const revokeMyToken = (id: string) =>
	request<void>(`/me/tokens/${encodeURIComponent(id)}`, { method: 'DELETE' });
