import { error } from '@sveltejs/kit';
import { ApiError } from '#lib/api/client.js';
import { listSecrets } from '#lib/api/secrets.js';
import { t } from '#lib/i18n/index.js';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, parent }) => {
	const { project } = await parent();
	const env = project.environments.find((e) => e.slug === params.env);
	if (!env) error(404, t.errors.notFound);
	try {
		return { env, secrets: await listSecrets(params.project, params.env) };
	} catch (e) {
		if (e instanceof ApiError && (e.status === 403 || e.status === 404))
			error(e.status, e.status === 403 ? t.errors.forbidden : t.errors.notFound);
		throw e;
	}
};
