// Thin fetch wrapper shared by every API module. It sends the session
// cookie, marks requests as coming from the web UI (required by the
// server's CSRF check), and turns error responses into ApiError.

import { goto } from '$app/navigation';

export class ApiError extends Error {
	status: number;
	code: string;
	details: Record<string, string>;

	constructor(status: number, code: string, message: string, details?: Record<string, string>) {
		super(message);
		this.status = status;
		this.code = code;
		this.details = details ?? {};
	}
}

export interface RequestOptions {
	method?: string;
	body?: unknown;
	/** Raw request body (sent as text/plain) instead of JSON. */
	text?: string;
	query?: Record<string, string | number | boolean | undefined>;
	/** Do not redirect on 401 or password_change_required. */
	quiet?: boolean;
}

// Pages that render without a session; a 401 there must not redirect.
const publicPaths = ['/login', '/setup'];

export function apiPath(path: string, query?: RequestOptions['query']): string {
	const url = '/api/v1' + path;
	if (!query) return url;
	const params = new URLSearchParams();
	for (const [k, v] of Object.entries(query)) {
		if (v !== undefined && v !== '') params.set(k, String(v));
	}
	const qs = params.toString();
	return qs ? `${url}?${qs}` : url;
}

export async function rawRequest(path: string, opts: RequestOptions = {}): Promise<Response> {
	const headers: Record<string, string> = { 'X-Envily-Client': 'web' };
	let body: string | undefined;
	if (opts.text !== undefined) {
		headers['Content-Type'] = 'text/plain; charset=utf-8';
		body = opts.text;
	} else if (opts.body !== undefined) {
		headers['Content-Type'] = 'application/json';
		body = JSON.stringify(opts.body);
	}
	let res: Response;
	try {
		res = await fetch(apiPath(path, opts.query), {
			method: opts.method ?? 'GET',
			headers,
			body,
			credentials: 'same-origin'
		});
	} catch {
		throw new ApiError(0, 'network', 'Cannot reach the server.');
	}
	if (res.ok) return res;

	let code = 'internal';
	let message = res.statusText || 'Request failed';
	let details: Record<string, string> | undefined;
	try {
		const data = (await res.json()) as {
			error?: { code: string; message: string; details?: Record<string, string> };
		};
		if (data.error) ({ code, message, details } = data.error);
	} catch {
		// Not a JSON error body; keep the defaults.
	}
	const err = new ApiError(res.status, code, message, details);
	if (!opts.quiet) redirectFor(err);
	throw err;
}

function redirectFor(err: ApiError) {
	const here = location.pathname;
	if (err.status === 401 && !publicPaths.includes(here)) {
		void goto('/login?next=' + encodeURIComponent(here + location.search), { replace: true });
	} else if (err.code === 'password_change_required' && here !== '/change-password') {
		void goto('/change-password', { replace: true });
	}
}

export async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
	const res = await rawRequest(path, opts);
	if (res.status === 204) return undefined as T;
	return (await res.json()) as T;
}
