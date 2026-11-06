import { error } from '@sveltejs/kit';
import { getProject } from '#lib/api/projects.js';
import { ApiError } from '#lib/api/client.js';
import { t } from '#lib/i18n/index.js';
import type { LayoutLoad } from './$types';

export const load: LayoutLoad = async ({ params, depends }) => {
	depends('app:project');
	try {
		return { project: await getProject(params.project) };
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) error(404, t.errors.notFound);
		throw e;
	}
};
