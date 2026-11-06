import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

// /p/{project} opens the first environment (or settings when there is none).
export const load: PageLoad = async ({ parent, params }) => {
	const { project } = await parent();
	const first = project.environments[0];
	redirect(307, first ? `/p/${params.project}/${first.slug}` : `/p/${params.project}/settings`);
};
