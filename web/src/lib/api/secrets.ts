import { rawRequest, request } from './client';
import type {
	BatchInput,
	BatchResult,
	ExportFormat,
	ImportMode,
	ImportResult,
	Secret,
	SecretItem,
	SecretVersion
} from './types';

const base = (project: string, env: string) =>
	`/projects/${encodeURIComponent(project)}/environments/${encodeURIComponent(env)}/secrets`;
const one = (project: string, env: string, key: string) =>
	`${base(project, env)}/${encodeURIComponent(key)}`;

export const listSecrets = (project: string, env: string, values = false) =>
	request<{ items: SecretItem[] }>(base(project, env), {
		query: { values: values || undefined }
	}).then((r) => r.items);

export const getSecret = (project: string, env: string, key: string) =>
	request<Secret>(one(project, env, key));

export const batchSecrets = (project: string, env: string, body: BatchInput) =>
	request<BatchResult>(`${base(project, env)}:batch`, { method: 'POST', body });

export const importSecrets = (project: string, env: string, text: string, mode: ImportMode) =>
	request<ImportResult>(`${base(project, env)}:import`, {
		method: 'POST',
		text,
		query: { mode }
	});

export async function exportSecrets(project: string, env: string, format: ExportFormat) {
	const res = await rawRequest(`${base(project, env)}:export`, { query: { format } });
	const disposition = res.headers.get('Content-Disposition') ?? '';
	const name = /filename="([^"]+)"/.exec(disposition)?.[1] ?? `${project}-${env}`;
	return { blob: await res.blob(), filename: name };
}

export const listVersions = (project: string, env: string, key: string, values = false) =>
	request<{ items: SecretVersion[] }>(`${one(project, env, key)}/versions`, {
		query: { values: values || undefined }
	}).then((r) => r.items);

export const rollbackSecret = (project: string, env: string, key: string, version: number) =>
	request<{ version: number }>(`${one(project, env, key)}/rollback`, {
		method: 'POST',
		body: { version }
	});
