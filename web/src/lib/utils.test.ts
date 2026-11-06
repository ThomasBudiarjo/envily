import { describe, expect, it } from 'vitest';
import { relativeTime, safeNext, slugify } from './utils';

describe('safeNext', () => {
	it('keeps same-origin paths', () => {
		expect(safeNext('/p/web-app/production?x=1')).toBe('/p/web-app/production?x=1');
	});
	it('rejects other origins and protocol-relative URLs', () => {
		for (const bad of [
			'https://evil.example',
			'//evil.example',
			'/\\evil.example',
			'javascript:alert(1)',
			'',
			null
		])
			expect(safeNext(bad)).toBe('/');
	});
});

describe('slugify', () => {
	it('matches the server rules', () => {
		expect(slugify('  My Web App! ')).toBe('my-web-app');
		expect(slugify('Über Café')).toBe('ber-caf');
		expect(slugify('a'.repeat(62) + ' b')).toBe('a'.repeat(62));
	});
});

describe('relativeTime', () => {
	const now = Date.parse('2026-11-06T12:00:00Z');
	it('rounds down to the largest unit', () => {
		expect(relativeTime('2026-11-06T11:59:31Z', now)).toBe('just now');
		expect(relativeTime('2026-11-06T11:15:00Z', now)).toBe('45 min ago');
		expect(relativeTime('2026-11-06T09:00:00Z', now)).toBe('3 h ago');
		expect(relativeTime('2026-11-01T12:00:00Z', now)).toBe('5 d ago');
	});
});
