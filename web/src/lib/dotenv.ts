// Client-side mirror of internal/dotenv.Parse, used only to preview an
// import before it is sent. The server parses the body again and is the
// source of truth.

export interface Entry {
	key: string;
	value: string;
}

/** Secret keys accepted by the server (env-var compatible). */
export const KEY_RE = /^[A-Za-z_][A-Za-z0-9_]{0,254}$/;

export const validKey = (key: string) => KEY_RE.test(key);

const DOTENV_KEY_RE = /^[A-Za-z_][A-Za-z0-9_.-]*$/;

export class ParseError extends Error {}

function closingQuote(s: string): number {
	for (let i = 0; i < s.length; i++) {
		if (s[i] === '\\') i++;
		else if (s[i] === '"') return i;
	}
	return -1;
}

function unescape(s: string): string {
	let out = '';
	for (let i = 0; i < s.length; i++) {
		if (s[i] === '\\' && i + 1 < s.length) {
			i++;
			const c = s[i];
			out += c === 'n' ? '\n' : c === 'r' ? '\r' : c === 't' ? '\t' : c;
			continue;
		}
		out += s[i];
	}
	return out;
}

/** Parses .env text. Errors name lines and keys, never values. */
export function parseDotenv(src: string): Entry[] {
	const lines = src.replaceAll('\r\n', '\n').split('\n');
	const out: Entry[] = [];
	const seen = new Map<string, number>();
	for (let i = 0; i < lines.length; i++) {
		let line = lines[i].trim();
		if (line === '' || line.startsWith('#')) continue;
		if (line.startsWith('export ')) line = line.slice(7);
		const eq = line.indexOf('=');
		if (eq < 0) throw new ParseError(`line ${i + 1}: missing '='`);
		const key = line.slice(0, eq).trim();
		if (!DOTENV_KEY_RE.test(key)) throw new ParseError(`line ${i + 1}: invalid key`);
		const first = seen.get(key);
		if (first !== undefined)
			throw new ParseError(`line ${i + 1}: duplicate key ${key} (first on line ${first})`);
		seen.set(key, i + 1);
		let raw = line.slice(eq + 1).replace(/^[ \t]+/, '');
		let value: string;
		if (raw.startsWith('"')) {
			let body = raw.slice(1);
			for (;;) {
				const end = closingQuote(body);
				if (end >= 0) {
					value = unescape(body.slice(0, end));
					break;
				}
				i++;
				if (i >= lines.length) throw new ParseError(`unterminated double quote for ${key}`);
				body += '\n' + lines[i];
			}
		} else if (raw.startsWith("'")) {
			const end = raw.indexOf("'", 1);
			if (end < 0) throw new ParseError(`line ${i + 1}: unterminated single quote`);
			value = raw.slice(1, end);
		} else {
			const idx = raw.indexOf(' #');
			if (idx >= 0) raw = raw.slice(0, idx);
			value = raw.trim();
		}
		out.push({ key, value });
	}
	return out;
}

/** Parses an import body the way the server does: a JSON object or .env text. */
export function parseImport(src: string): Entry[] {
	const trimmed = src.trim();
	if (trimmed.startsWith('{')) {
		let obj: unknown;
		try {
			obj = JSON.parse(trimmed);
		} catch {
			throw new ParseError('not a valid JSON object');
		}
		if (typeof obj !== 'object' || obj === null || Array.isArray(obj))
			throw new ParseError('not a valid JSON object');
		const out: Entry[] = [];
		for (const [key, value] of Object.entries(obj)) {
			if (typeof value !== 'string') throw new ParseError(`${key}: value must be a string`);
			out.push({ key, value });
		}
		return out.sort((a, b) => (a.key < b.key ? -1 : a.key > b.key ? 1 : 0));
	}
	return parseDotenv(src);
}

export interface ImportPreview {
	added: string[];
	changed: string[];
	removed: string[];
	invalid: string[];
}

/**
 * Previews what an import changes. `current` maps live keys to their values
 * when known; keys whose values are unknown count as changed.
 */
export function previewImport(
	entries: Entry[],
	current: Map<string, string | undefined>,
	mode: 'merge' | 'overwrite'
): ImportPreview {
	const p: ImportPreview = { added: [], changed: [], removed: [], invalid: [] };
	const incoming = new Set<string>();
	for (const e of entries) {
		incoming.add(e.key);
		if (!validKey(e.key)) p.invalid.push(e.key);
		else if (!current.has(e.key)) p.added.push(e.key);
		else if (current.get(e.key) !== e.value) p.changed.push(e.key);
	}
	if (mode === 'overwrite') {
		for (const key of current.keys()) if (!incoming.has(key)) p.removed.push(key);
	}
	return p;
}
