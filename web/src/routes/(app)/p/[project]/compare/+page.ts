import { compare } from '#lib/api/projects.js';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params }) => ({
	comparison: await compare(params.project)
});
