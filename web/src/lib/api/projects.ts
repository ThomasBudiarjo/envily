import { request } from './client';
import type { Comparison, Environment, Member, Project, Role } from './types';

const p = (slug: string) => `/projects/${encodeURIComponent(slug)}`;

export const listProjects = () => request<{ items: Project[] }>('/projects').then((r) => r.items);

export const getProject = (slug: string) => request<Project>(p(slug));

export const createProject = (name: string, slug?: string) =>
	request<Project>('/projects', { method: 'POST', body: { name, slug: slug || undefined } });

export const updateProject = (slug: string, body: { name?: string; slug?: string }) =>
	request<Project>(p(slug), { method: 'PATCH', body });

export const deleteProject = (slug: string) =>
	request<void>(p(slug), { method: 'DELETE', body: { confirm: slug } });

export const listMembers = (slug: string) =>
	request<{ items: Member[] }>(`${p(slug)}/members`).then((r) => r.items);

export const addMember = (slug: string, body: { userId?: string; email?: string; role: Role }) =>
	request<Member>(`${p(slug)}/members`, { method: 'POST', body });

export const updateMember = (slug: string, userId: string, role: Role) =>
	request<Member>(`${p(slug)}/members/${encodeURIComponent(userId)}`, {
		method: 'PATCH',
		body: { role }
	});

export const removeMember = (slug: string, userId: string) =>
	request<void>(`${p(slug)}/members/${encodeURIComponent(userId)}`, { method: 'DELETE' });

export const listEnvironments = (slug: string) =>
	request<{ items: Environment[] }>(`${p(slug)}/environments`).then((r) => r.items);

export const createEnvironment = (slug: string, name: string, envSlug?: string) =>
	request<Environment>(`${p(slug)}/environments`, {
		method: 'POST',
		body: { name, slug: envSlug || undefined }
	});

export const updateEnvironment = (
	slug: string,
	env: string,
	body: { name?: string; slug?: string; position?: number }
) =>
	request<Environment>(`${p(slug)}/environments/${encodeURIComponent(env)}`, {
		method: 'PATCH',
		body
	});

export const deleteEnvironment = (slug: string, env: string) =>
	request<void>(`${p(slug)}/environments/${encodeURIComponent(env)}`, {
		method: 'DELETE',
		body: { confirm: env }
	});

export const compare = (slug: string, envs?: string[]) =>
	request<Comparison>(`${p(slug)}/compare`, { query: { envs: envs?.join(',') } });
