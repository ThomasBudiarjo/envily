import { t } from './i18n';

/** Joins class names, skipping falsy values. */
export function cn(...parts: (string | false | null | undefined)[]): string {
	return parts.filter(Boolean).join(' ');
}

/** "3 min ago" style text for an ISO time. */
export function relativeTime(iso: string, now = Date.now()): string {
	const diff = Math.max(0, now - Date.parse(iso));
	const min = Math.floor(diff / 60_000);
	if (min < 1) return t.time.justNow;
	if (min < 60) return t.time.minutes(min);
	const h = Math.floor(min / 60);
	if (h < 24) return t.time.hours(h);
	const d = Math.floor(h / 24);
	if (d < 30) return t.time.days(d);
	return new Date(iso).toLocaleDateString();
}

export function absoluteTime(iso: string): string {
	return new Date(iso).toLocaleString();
}

export const SLUG_RE = /^[a-z0-9][a-z0-9-]{0,62}$/;

/** Suggests a slug from a display name, like the server does. */
export function slugify(name: string): string {
	return name
		.toLowerCase()
		.normalize('NFKD')
		.replace(/[^a-z0-9]+/g, '-')
		.replace(/^-+|-+$/g, '')
		.slice(0, 63);
}

export function downloadBlob(blob: Blob, filename: string) {
	const url = URL.createObjectURL(blob);
	const a = document.createElement('a');
	a.href = url;
	a.download = filename;
	a.click();
	setTimeout(() => URL.revokeObjectURL(url), 1000);
}

/** ISO time `days` days from now, or undefined for "never". */
export function expiryFromDays(days: number): string | undefined {
	return days > 0 ? new Date(Date.now() + days * 86_400_000).toISOString() : undefined;
}
