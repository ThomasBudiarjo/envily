import { describe, expect, it } from 'vitest';
import type { SecretItem } from '#lib/api/types.js';
import { buildBatch, fromServer, newRow, pendingCount, rowState, validate } from './draft';

const item = (key: string, version: number, extra: Partial<SecretItem> = {}): SecretItem => ({
	key,
	version,
	createdAt: '2026-01-01T00:00:00Z',
	updatedAt: '2026-01-01T00:00:00Z',
	updatedBy: { type: 'user', id: 'u', label: 'u@example.com' },
	hasComment: false,
	...extra
});

describe('draft rows', () => {
	it('starts clean and produces an empty batch', () => {
		const rows = fromServer([item('A', 1), item('B', 3)]);
		expect(rows.map(rowState)).toEqual(['clean', 'clean']);
		expect(pendingCount(rows)).toBe(0);
		expect(buildBatch(rows)).toEqual({ upserts: [], deletes: [], renames: [] });
	});

	it('loading a value is not a change; editing it is', () => {
		const [r] = fromServer([item('A', 2)]);
		r.value = 'loaded';
		r.loadedValue = 'loaded';
		expect(rowState(r)).toBe('clean');
		r.value = 'edited';
		expect(rowState(r)).toBe('changed');
		expect(buildBatch([r]).upserts).toEqual([{ key: 'A', value: 'edited', expectedVersion: 2 }]);
	});

	it('sends the comment only when it changed', () => {
		const [r] = fromServer([item('A', 4, { value: 'v', comment: 'old' })]);
		r.comment = 'new';
		expect(buildBatch([r]).upserts).toEqual([
			{ key: 'A', value: 'v', comment: 'new', expectedVersion: 4 }
		]);
	});

	it('renames by original key and upserts under the new key', () => {
		const [r] = fromServer([item('OLD', 5, { value: 'v', comment: '' })]);
		r.key = 'NEW';
		expect(buildBatch([r])).toEqual({
			upserts: [],
			deletes: [],
			renames: [{ from: 'OLD', to: 'NEW' }]
		});
		r.value = 'v2';
		expect(buildBatch([r]).upserts).toEqual([{ key: 'NEW', value: 'v2', expectedVersion: 5 }]);
	});

	it('deletes by the original key even after a rename', () => {
		const [r] = fromServer([item('OLD', 1)]);
		r.key = 'RENAMED';
		r.deleted = true;
		expect(rowState(r)).toBe('deleted');
		expect(buildBatch([r])).toEqual({ upserts: [], deletes: ['OLD'], renames: [] });
	});

	it('new rows expect version 0; a deleted new row is dropped', () => {
		const a = newRow();
		a.key = 'FRESH';
		a.value = 'x';
		const b = newRow();
		b.key = 'GONE';
		b.deleted = true;
		expect(buildBatch([a, b]).upserts).toEqual([{ key: 'FRESH', value: 'x', expectedVersion: 0 }]);
		expect(pendingCount([a, b])).toBe(1);
	});
});

describe('validate', () => {
	it('flags invalid and duplicate keys on edited rows only', () => {
		const rows = fromServer([item('A', 1), item('B', 1)]);
		const n = newRow();
		n.key = 'A';
		const bad = newRow();
		bad.key = '1BAD';
		const errors = validate([...rows, n, bad]);
		expect(errors.get(n.id)).toMatch(/already exists/);
		expect(errors.get(bad.id)).toMatch(/letters/);
		expect(errors.has(rows[0].id)).toBe(false);
	});

	it('allows reusing a key that is deleted in the same draft', () => {
		const rows = fromServer([item('A', 1)]);
		rows[0].deleted = true;
		const n = newRow();
		n.key = 'A';
		expect(validate([...rows, n]).size).toBe(0);
	});

	it('limits value size in bytes, not characters', () => {
		const n = newRow();
		n.key = 'BIG';
		n.value = 'é'.repeat(32 * 1024 + 1); // 2 bytes each → just over 64 KiB
		expect(validate([n]).get(n.id)).toMatch(/64 KiB/);
	});
});
