import { describe, expect, it } from 'vitest';
import { parseDotenv, parseImport, previewImport, validKey } from './dotenv';

describe('parseDotenv', () => {
	it('handles comments, export, quotes, and inline comments', () => {
		const src = [
			'# comment',
			'export A=1',
			"B='literal \\n #x'",
			'C="line1\\nline2"',
			'D=plain # trailing',
			'E="multi',
			'line"',
			'F='
		].join('\r\n');
		expect(parseDotenv(src)).toEqual([
			{ key: 'A', value: '1' },
			{ key: 'B', value: 'literal \\n #x' },
			{ key: 'C', value: 'line1\nline2' },
			{ key: 'D', value: 'plain' },
			{ key: 'E', value: 'multi\nline' },
			{ key: 'F', value: '' }
		]);
	});

	it('keeps # inside a value without a leading space', () => {
		expect(parseDotenv('URL=http://x/#frag')).toEqual([{ key: 'URL', value: 'http://x/#frag' }]);
	});

	it('rejects duplicates without quoting values', () => {
		expect(() => parseDotenv('A=s3cret\nA=other')).toThrow(/line 2: duplicate key A/);
		try {
			parseDotenv('A=s3cret\nA=other');
		} catch (e) {
			expect(String(e)).not.toContain('s3cret');
		}
	});

	it('reports unterminated quotes', () => {
		expect(() => parseDotenv('A="open')).toThrow(/unterminated double quote/);
		expect(() => parseDotenv("A='open")).toThrow(/line 1: unterminated single quote/);
	});
});

describe('parseImport', () => {
	it('accepts a JSON object of strings, sorted by key', () => {
		expect(parseImport(' {"B":"2","A":"1"} ')).toEqual([
			{ key: 'A', value: '1' },
			{ key: 'B', value: '2' }
		]);
		expect(() => parseImport('{"A":1}')).toThrow(/must be a string/);
		expect(() => parseImport('{nope')).toThrow(/JSON/);
	});
});

describe('validKey', () => {
	it('matches the server regex', () => {
		expect(validKey('_A1')).toBe(true);
		expect(validKey('1A')).toBe(false);
		expect(validKey('A-B')).toBe(false);
		expect(validKey('A'.repeat(255))).toBe(true);
		expect(validKey('A'.repeat(256))).toBe(false);
	});
});

describe('previewImport', () => {
	const current = new Map<string, string | undefined>([
		['KEEP', 'same'],
		['EDIT', 'old'],
		['UNKNOWN', undefined],
		['GONE', 'x']
	]);
	const entries = [
		{ key: 'KEEP', value: 'same' },
		{ key: 'EDIT', value: 'new' },
		{ key: 'UNKNOWN', value: 'v' },
		{ key: 'NEW', value: '1' },
		{ key: 'bad.key', value: '1' }
	];

	it('merge never removes', () => {
		expect(previewImport(entries, current, 'merge')).toEqual({
			added: ['NEW'],
			changed: ['EDIT', 'UNKNOWN'],
			removed: [],
			invalid: ['bad.key']
		});
	});

	it('overwrite removes keys missing from the input', () => {
		expect(previewImport(entries, current, 'overwrite').removed).toEqual(['GONE']);
	});
});
