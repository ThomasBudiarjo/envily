import { ApiError } from './api/client';
import { t } from './i18n';

/** Human message for an API error, for toasts and form banners. */
export function errorMessage(e: unknown): string {
	if (e instanceof ApiError) {
		if (e.code === 'network') return t.errors.network;
		if (e.code === 'forbidden') return t.errors.forbidden;
		if (e.code === 'last_owner') return t.settings.lastOwner;
		if (e.code === 'rate_limited') return t.auth.rateLimited;
		const first = Object.entries(e.details)[0];
		if (e.code === 'validation_failed' && first) return `${first[0]} ${first[1]}`;
		return e.message;
	}
	return t.common.error;
}

/** Field errors from a validation_failed response. */
export function fieldErrors(e: unknown): Record<string, string> {
	return e instanceof ApiError && e.code === 'validation_failed' ? e.details : {};
}
