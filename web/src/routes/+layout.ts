import { redirect } from '@sveltejs/kit';
import { session } from '#lib/stores/session.svelte.js';
import { safeNext } from '#lib/utils.js';
import type { LayoutLoad } from './$types';

// The UI is a client-rendered SPA served from the Go binary.
export const ssr = false;
export const prerender = false;

const publicPaths = ['/login', '/setup'];

// Route guard: setup first, then sign-in, then a forced password change.
// The session is fetched once; later navigations reuse it.
export const load: LayoutLoad = async ({ url }) => {
	if (!session.loaded) await session.load();
	const path = url.pathname;
	if (session.needsSetup) {
		if (path !== '/setup') redirect(307, '/setup');
		return;
	}
	if (path === '/setup') redirect(307, session.me ? '/' : '/login');
	if (!session.me) {
		if (!publicPaths.includes(path))
			redirect(307, '/login?next=' + encodeURIComponent(path + url.search));
		return;
	}
	if (publicPaths.includes(path)) redirect(307, safeNext(url.searchParams.get('next')));
	if (session.me.mustChangePassword && path !== '/change-password')
		redirect(307, '/change-password');
};
