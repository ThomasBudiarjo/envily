import { request } from './client';
import type { AuditEntry, AuditFilter, Page } from './types';

export const instanceAudit = (f: AuditFilter = {}) =>
	request<Page<AuditEntry>>('/audit', { query: { ...f } });

export const projectAudit = (project: string, f: AuditFilter = {}) =>
	request<Page<AuditEntry>>(`/projects/${encodeURIComponent(project)}/audit`, {
		query: { ...f }
	});
