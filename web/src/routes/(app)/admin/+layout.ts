import { error } from '@sveltejs/kit';
import { t } from '#lib/i18n/index.js';
import { session } from '#lib/stores/session.svelte.js';
import type { LayoutLoad } from './$types';

export const load: LayoutLoad = async ({ parent }) => {
	// The root layout loads the session; wait for it before checking.
	await parent();
	if (!session.isAdmin) error(404, t.errors.notFound);
};
