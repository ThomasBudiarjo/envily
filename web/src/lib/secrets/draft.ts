// Draft model behind the secret table. Rows start from the server list;
// edits, additions, renames, and deletions stay local until saved as one
// atomic POST …/secrets:batch.

import { validKey } from '#lib/dotenv.js';
import { t } from '#lib/i18n/index.js';
import type { BatchInput, SecretItem } from '#lib/api/types.js';

export const MAX_VALUE_BYTES = 64 * 1024;
export const MAX_COMMENT_BYTES = 1024;

export interface Row {
	/** Stable client-side id (the original key, or a generated id for new rows). */
	id: string;
	/** The secret as last loaded from the server; null for new rows. */
	original: SecretItem | null;
	key: string;
	/** Known plaintext value (loaded or typed); undefined when not loaded. */
	value?: string;
	/** Value as loaded from the server, used to detect real changes. */
	loadedValue?: string;
	comment?: string;
	loadedComment?: string;
	deleted: boolean;
}

export type RowState = 'clean' | 'new' | 'changed' | 'deleted';

let seq = 0;

export function fromServer(items: SecretItem[]): Row[] {
	return items.map((s) => ({
		id: 'k:' + s.key,
		original: s,
		key: s.key,
		value: s.value,
		loadedValue: s.value,
		comment: s.comment,
		loadedComment: s.comment,
		deleted: false
	}));
}

export function newRow(): Row {
	return { id: 'n:' + ++seq, original: null, key: '', value: '', comment: '', deleted: false };
}

const valueChanged = (r: Row) => r.value !== undefined && r.value !== r.loadedValue;
const commentChanged = (r: Row) => r.comment !== undefined && r.comment !== r.loadedComment;

export function rowState(r: Row): RowState {
	if (r.original === null) return 'new';
	if (r.deleted) return 'deleted';
	if (r.key !== r.original.key || valueChanged(r) || commentChanged(r)) return 'changed';
	return 'clean';
}

/** Number of pending changes (new rows that are deleted again do not count). */
export function pendingCount(rows: Row[]): number {
	return rows.filter((r) => rowState(r) !== 'clean' && !(r.original === null && r.deleted)).length;
}

const bytes = (s: string) => new TextEncoder().encode(s).length;

/** Validation errors keyed by row id. */
export function validate(rows: Row[]): Map<string, string> {
	const errors = new Map<string, string>();
	const live = rows.filter((r) => !r.deleted);
	const counts = new Map<string, number>();
	for (const r of live) counts.set(r.key, (counts.get(r.key) ?? 0) + 1);
	for (const r of live) {
		if (rowState(r) === 'clean') continue;
		if (!validKey(r.key)) errors.set(r.id, t.secrets.invalidKey);
		else if ((counts.get(r.key) ?? 0) > 1) errors.set(r.id, t.secrets.duplicateKey);
		else if (r.value !== undefined && bytes(r.value) > MAX_VALUE_BYTES)
			errors.set(r.id, t.secrets.valueTooLarge);
		else if (r.comment !== undefined && bytes(r.comment) > MAX_COMMENT_BYTES)
			errors.set(r.id, t.secrets.commentTooLarge);
	}
	// A duplicate with a clean row is reported on the edited row only.
	return errors;
}

/**
 * Builds the batch body. The server applies deletes, then renames, then
 * upserts. Upserts carry expectedVersion so a concurrent change gives 409
 * instead of being overwritten; new keys expect version 0 (absent).
 */
export function buildBatch(rows: Row[]): BatchInput {
	const batch: BatchInput = { upserts: [], deletes: [], renames: [] };
	for (const r of rows) {
		const state = rowState(r);
		if (state === 'clean') continue;
		if (r.original === null) {
			if (r.deleted) continue;
			batch.upserts.push({
				key: r.key,
				value: r.value ?? '',
				comment: r.comment || undefined,
				expectedVersion: 0
			});
			continue;
		}
		if (state === 'deleted') {
			batch.deletes.push(r.original.key);
			continue;
		}
		if (r.key !== r.original.key) batch.renames.push({ from: r.original.key, to: r.key });
		if (valueChanged(r) || commentChanged(r)) {
			// An upsert replaces the value, so it must be known; the table loads
			// it before allowing edits. Never fall back to an empty string.
			if (r.value === undefined) throw new Error(`value of ${r.key} is not loaded`);
			const up: BatchInput['upserts'][number] = {
				key: r.key,
				value: r.value,
				expectedVersion: r.original.version
			};
			if (commentChanged(r)) up.comment = r.comment;
			batch.upserts.push(up);
		}
	}
	return batch;
}

/** Maps conflict details (keyed by secret key) to the rows they name. */
export function conflictRows(rows: Row[], details: Record<string, string>): Row[] {
	const keys = new Set(Object.keys(details));
	return rows.filter((r) => keys.has(r.key) || (r.original && keys.has(r.original.key)));
}
