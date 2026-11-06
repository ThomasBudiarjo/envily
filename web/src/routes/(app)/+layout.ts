import { listProjects } from '#lib/api/projects.js';
import type { LayoutLoad } from './$types';

export const load: LayoutLoad = async ({ depends }) => {
	depends('app:projects');
	return { projects: await listProjects() };
};
