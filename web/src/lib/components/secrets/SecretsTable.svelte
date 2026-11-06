<script lang="ts">
	import { beforeNavigate } from '$app/navigation';
	import Copy from '@lucide/svelte/icons/copy';
	import Download from '@lucide/svelte/icons/download';
	import Ellipsis from '@lucide/svelte/icons/ellipsis';
	import Eye from '@lucide/svelte/icons/eye';
	import EyeOff from '@lucide/svelte/icons/eye-off';
	import FileUp from '@lucide/svelte/icons/file-up';
	import History from '@lucide/svelte/icons/history';
	import KeyRound from '@lucide/svelte/icons/key-round';
	import Lock from '@lucide/svelte/icons/lock';
	import MessageSquare from '@lucide/svelte/icons/message-square';
	import Pencil from '@lucide/svelte/icons/pencil';
	import Plus from '@lucide/svelte/icons/plus';
	import Search from '@lucide/svelte/icons/search';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import Undo2 from '@lucide/svelte/icons/undo-2';
	import { onDestroy, tick } from 'svelte';
	import { toast } from 'svelte-sonner';
	import { SvelteSet } from 'svelte/reactivity';
	import { ApiError } from '#lib/api/client.js';
	import { batchSecrets, exportSecrets, getSecret, listSecrets } from '#lib/api/secrets.js';
	import type { ExportFormat, SecretItem } from '#lib/api/types.js';
	import Badge from '#lib/components/ui/Badge.svelte';
	import Button from '#lib/components/ui/Button.svelte';
	import Dialog from '#lib/components/ui/Dialog.svelte';
	import DropdownMenu, { type MenuItem } from '#lib/components/ui/DropdownMenu.svelte';
	import EmptyState from '#lib/components/ui/EmptyState.svelte';
	import IconButton from '#lib/components/ui/IconButton.svelte';
	import Input from '#lib/components/ui/Input.svelte';
	import Textarea from '#lib/components/ui/Textarea.svelte';
	import Tooltip from '#lib/components/ui/Tooltip.svelte';
	import { errorMessage } from '#lib/forms.js';
	import { t } from '#lib/i18n/index.js';
	import {
		buildBatch,
		fromServer,
		newRow,
		pendingCount,
		rebase,
		rowState,
		validate,
		type Row
	} from '#lib/secrets/draft.js';
	import { absoluteTime, cn, downloadBlob, relativeTime } from '#lib/utils.js';
	import HistorySheet from './HistorySheet.svelte';
	import ImportDialog from './ImportDialog.svelte';

	let {
		project,
		env,
		envName,
		initial,
		canWrite,
		canHistoryValues
	}: {
		project: string;
		env: string;
		envName: string;
		initial: SecretItem[];
		/** Editors and owners (and read_write tokens) may change secrets. */
		canWrite: boolean;
		/** Editors and owners may see old values and roll back. */
		canHistoryValues: boolean;
	} = $props();

	const HIDE_AFTER_MS = 60_000;

	// svelte-ignore state_referenced_locally
	let rows = $state<Row[]>(fromServer(initial));
	let query = $state('');
	let attempted = $state(false);
	let saving = $state(false);
	let editing = $state<string | null>(null);
	let renaming = $state<string | null>(null);
	const revealed = new SvelteSet<string>();
	// Timer handles are bookkeeping, not UI state, so a plain Map is intended.
	// eslint-disable-next-line svelte/prefer-svelte-reactivity
	const timers = new Map<string, ReturnType<typeof setTimeout>>();

	let importOpen = $state(false);
	let historyKey = $state('');
	let historyOpen = $state(false);
	let commentRow = $state<Row | null>(null);
	let commentDraft = $state('');
	let commentOpen = $state(false);
	let conflicts = $state<Record<string, string>>({});
	let conflictOpen = $state(false);

	const pending = $derived(pendingCount(rows));
	const errors = $derived(validate(rows));
	const live = $derived(rows.filter((r) => r.original !== null));
	const visible = $derived.by(() => {
		const q = query.trim().toLowerCase();
		return q ? rows.filter((r) => r.key.toLowerCase().includes(q) || r.original === null) : rows;
	});
	const allRevealed = $derived(live.length > 0 && live.every((r) => revealed.has(r.id)));
	const currentValues = $derived(
		new Map(live.map((r) => [r.original!.key, r.loadedValue] as [string, string | undefined]))
	);

	function showError(r: Row) {
		const e = errors.get(r.id);
		return e && (attempted || r.key !== '') ? e : '';
	}

	// ---- loading and revealing values ----

	async function ensureLoaded(r: Row): Promise<boolean> {
		if (r.value !== undefined || r.original === null) return true;
		try {
			const s = await getSecret(project, env, r.original.key);
			r.value = r.loadedValue = s.value;
			r.comment = r.loadedComment = s.comment;
			return true;
		} catch (e) {
			toast.error(errorMessage(e));
			return false;
		}
	}

	function scheduleHide(r: Row) {
		clearTimeout(timers.get(r.id));
		timers.set(
			r.id,
			setTimeout(() => hide(r), HIDE_AFTER_MS)
		);
	}

	function hide(r: Row) {
		clearTimeout(timers.get(r.id));
		timers.delete(r.id);
		if (editing === r.id) return;
		revealed.delete(r.id);
		// Forget plaintext we no longer show, unless it is part of a draft.
		if (rowState(r) === 'clean') {
			r.value = r.loadedValue = undefined;
			r.comment = r.loadedComment = undefined;
		}
	}

	async function reveal(r: Row) {
		if (!(await ensureLoaded(r))) return;
		revealed.add(r.id);
		scheduleHide(r);
	}

	async function toggleAll() {
		if (allRevealed) {
			for (const r of live) hide(r);
			return;
		}
		try {
			const items = await listSecrets(project, env, true);
			const byKey = new Map(items.map((s) => [s.key, s]));
			for (const r of live) {
				const s = byKey.get(r.original!.key);
				if (s && r.value === undefined) {
					r.value = r.loadedValue = s.value;
					r.comment = r.loadedComment = s.comment;
				}
				if (r.value !== undefined) {
					revealed.add(r.id);
					scheduleHide(r);
				}
			}
		} catch (e) {
			toast.error(errorMessage(e));
		}
	}

	async function copy(r: Row) {
		if (!(await ensureLoaded(r))) return;
		try {
			await navigator.clipboard.writeText(r.value ?? '');
			toast.success(t.common.copied);
		} catch {
			toast.error(t.common.error);
		}
		if (!revealed.has(r.id) && rowState(r) === 'clean') hide(r);
	}

	// ---- editing ----

	async function startEdit(r: Row) {
		if (!canWrite || r.deleted) return;
		if (!(await ensureLoaded(r))) return;
		editing = r.id;
		revealed.add(r.id);
		clearTimeout(timers.get(r.id));
		await tick();
		document.querySelector<HTMLTextAreaElement>(`[data-value-editor="${r.id}"]`)?.focus();
	}

	function stopEdit(r: Row) {
		if (editing === r.id) editing = null;
		scheduleHide(r);
	}

	async function addRow() {
		const r = newRow();
		rows.unshift(r);
		query = '';
		await tick();
		document.querySelector<HTMLInputElement>(`[data-key-editor="${r.id}"]`)?.focus();
	}

	async function startRename(r: Row) {
		renaming = r.id;
		await tick();
		document.querySelector<HTMLInputElement>(`[data-key-editor="${r.id}"]`)?.select();
	}

	async function editComment(r: Row) {
		if (!(await ensureLoaded(r))) return;
		commentRow = r;
		commentDraft = r.comment ?? '';
		commentOpen = true;
	}

	function applyComment(e: SubmitEvent) {
		e.preventDefault();
		if (commentRow) commentRow.comment = commentDraft;
		commentOpen = false;
	}

	function toggleDelete(r: Row) {
		if (r.original === null) rows = rows.filter((x) => x !== r);
		else r.deleted = !r.deleted;
		if (editing === r.id) editing = null;
	}

	function undo(r: Row) {
		if (r.original === null) return toggleDelete(r);
		r.key = r.original.key;
		r.value = r.loadedValue;
		r.comment = r.loadedComment;
		r.deleted = false;
		if (renaming === r.id) renaming = null;
	}

	// ---- saving ----

	async function reload(drop = new Set<string>()) {
		const fresh = await listSecrets(project, env);
		for (const r of rows) clearTimeout(timers.get(r.id));
		timers.clear();
		revealed.clear();
		editing = renaming = null;
		rows = drop.size || pending ? rebase(rows, fresh, drop) : fromServer(fresh);
	}

	function discard() {
		rows = fromServer(rows.filter((r) => r.original).map((r) => r.original!));
		revealed.clear();
		editing = renaming = null;
		attempted = false;
	}

	async function save() {
		attempted = true;
		if (errors.size) {
			toast.error([...errors.values()][0]);
			return;
		}
		saving = true;
		try {
			const res = await batchSecrets(project, env, buildBatch(rows));
			toast.success(t.secrets.saved(res.created + res.updated + res.deleted + res.renamed));
			attempted = false;
			rows = fromServer(await listSecrets(project, env));
			revealed.clear();
			editing = renaming = null;
		} catch (e) {
			if (e instanceof ApiError && e.code === 'conflict') {
				conflicts = e.details;
				conflictOpen = true;
			} else toast.error(errorMessage(e));
		} finally {
			saving = false;
		}
	}

	async function reloadAfterConflict() {
		try {
			await reload(new Set(Object.keys(conflicts)));
			conflictOpen = false;
		} catch (e) {
			toast.error(errorMessage(e));
		}
	}

	async function exportAs(format: ExportFormat) {
		try {
			const { blob, filename } = await exportSecrets(project, env, format);
			downloadBlob(blob, filename);
		} catch (e) {
			toast.error(errorMessage(e));
		}
	}

	function openHistory(r: Row) {
		historyKey = r.original!.key;
		historyOpen = true;
	}

	// Leaving with drafts asks first; unloading the page uses the browser prompt.
	beforeNavigate(({ cancel, willUnload }) => {
		if (!pending) return;
		if (willUnload) cancel();
		else if (!confirm(t.secrets.unsavedWarning)) cancel();
	});

	onDestroy(() => {
		for (const id of timers.keys()) clearTimeout(timers.get(id));
	});

	const stateTone = { new: 'success', changed: 'warning', deleted: 'destructive' } as const;
	const stateLabel = {
		new: t.secrets.markNew,
		changed: t.secrets.markChanged,
		deleted: t.secrets.markDeleted
	};

	function menuFor(r: Row): MenuItem[] {
		const items: MenuItem[] = [
			{ label: t.secrets.copyValue, icon: Copy, onselect: () => copy(r) },
			{ label: t.secrets.history, icon: History, onselect: () => openHistory(r) }
		];
		if (canWrite) {
			items.push(
				{ label: t.common.rename, icon: Pencil, onselect: () => startRename(r) },
				{ label: t.secrets.comment, icon: MessageSquare, onselect: () => editComment(r) }
			);
			const del: MenuItem = r.deleted
				? { label: t.secrets.undo, icon: Undo2, onselect: () => undo(r), separatorBefore: true }
				: {
						label: t.secrets.deleteSecret,
						icon: Trash2,
						onselect: () => toggleDelete(r),
						destructive: true,
						separatorBefore: true
					};
			items.push(del);
		}
		return items;
	}
</script>

<div class="grid gap-4 pb-24">
	<div class="flex flex-wrap items-center gap-2">
		<div class="mr-auto min-w-0">
			<h1 class="truncate text-xl font-semibold tracking-tight">{envName}</h1>
			<p class="text-[13px] text-muted-foreground" data-testid="secret-count">
				{t.secrets.count(live.length)}
			</p>
		</div>
		{#if live.length}
			<Button variant="outline" size="sm" onclick={toggleAll}>
				{#if allRevealed}<EyeOff />{t.secrets.hideAll}{:else}<Eye />{t.secrets.showAll}{/if}
			</Button>
		{/if}
		{#if canWrite}
			<Button
				variant="outline"
				size="sm"
				onclick={() => (importOpen = true)}
				disabled={pending > 0}
			>
				<FileUp />{t.secrets.import}
			</Button>
		{/if}
		{#if live.length}
			<DropdownMenu
				items={(['dotenv', 'json', 'yaml'] as const).map((f) => ({
					label: t.secrets.exportFormat[f],
					onselect: () => exportAs(f)
				}))}
			>
				{#snippet trigger({ props })}
					<Button {...props} variant="outline" size="sm"><Download />{t.secrets.export}</Button>
				{/snippet}
			</DropdownMenu>
		{/if}
		{#if canWrite}
			<Button size="sm" onclick={addRow}><Plus />{t.secrets.add}</Button>
		{/if}
	</div>

	{#if !canWrite}
		<p
			class="flex items-center gap-2 rounded-md border bg-muted/40 px-3 py-2 text-[13px] text-muted-foreground"
		>
			<Lock class="size-4 shrink-0" />{t.secrets.readOnly}
		</p>
	{/if}

	{#if rows.length === 0}
		<EmptyState icon={KeyRound} title={t.secrets.empty} description={t.secrets.emptyIntro}>
			{#if canWrite}
				<Button onclick={addRow}><Plus />{t.secrets.addFirst}</Button>
				<Button variant="outline" onclick={() => (importOpen = true)}>
					<FileUp />{t.secrets.importFromEnv}
				</Button>
			{/if}
		</EmptyState>
	{:else}
		<div class="relative">
			<Search
				class="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground"
			/>
			<Input
				bind:value={query}
				placeholder={t.secrets.searchPlaceholder}
				aria-label={t.common.search}
				class="pl-9"
				type="search"
			/>
		</div>

		<div class="overflow-hidden rounded-lg border bg-card" role="table" aria-label={envName}>
			<div
				role="row"
				class="hidden border-b bg-muted/40 px-3 py-2 text-xs font-medium text-muted-foreground md:grid md:grid-cols-[minmax(11rem,2fr)_minmax(0,3fr)_2rem_7rem_2.25rem] md:gap-3"
			>
				<span role="columnheader">{t.secrets.key}</span>
				<span role="columnheader">{t.secrets.value}</span>
				<span role="columnheader" class="sr-only">{t.secrets.comment}</span>
				<span role="columnheader">{t.secrets.updated}</span>
				<span role="columnheader" class="sr-only">{t.common.actions}</span>
			</div>
			{#each visible as r (r.id)}
				{@const st = rowState(r)}
				{@const err = showError(r)}
				{@const isOpen = revealed.has(r.id) || editing === r.id || r.original === null}
				<div
					role="row"
					data-testid="secret-row"
					data-key={r.key}
					data-state={st}
					class={cn(
						'relative grid grid-cols-[minmax(0,1fr)_auto] gap-x-2 gap-y-1.5 border-b px-3 py-2.5 last:border-b-0 md:grid-cols-[minmax(11rem,2fr)_minmax(0,3fr)_2rem_7rem_2.25rem] md:items-start md:gap-3',
						st === 'new' && 'bg-success/6',
						st === 'changed' && 'bg-warning/7',
						st === 'deleted' && 'bg-destructive/6',
						err && 'ring-1 ring-destructive ring-inset'
					)}
				>
					{#if st !== 'clean'}
						<span
							class={cn(
								'absolute inset-y-0 left-0 w-0.5',
								st === 'new' && 'bg-success',
								st === 'changed' && 'bg-warning',
								st === 'deleted' && 'bg-destructive'
							)}
						></span>
					{/if}
					<!-- key -->
					<div role="cell" class="flex min-w-0 items-center gap-2 md:min-h-8">
						{#if canWrite && (r.original === null || renaming === r.id)}
							<Input
								bind:value={r.key}
								data-key-editor={r.id}
								placeholder={t.secrets.keyPlaceholder}
								aria-label={t.secrets.key}
								invalid={!!err}
								class="h-8 font-mono text-[13px]"
								autocomplete="off"
								spellcheck={false}
								onblur={() => {
									if (renaming === r.id) renaming = null;
								}}
							/>
						{:else}
							<span
								class={cn(
									'truncate font-mono text-[13px] font-medium',
									st === 'deleted' && 'text-muted-foreground line-through'
								)}
								title={r.key}>{r.key}</span
							>
						{/if}
						{#if st !== 'clean'}
							<Badge tone={stateTone[st]} class="shrink-0">{stateLabel[st]}</Badge>
						{/if}
					</div>
					<!-- actions (top right on mobile) -->
					<div role="cell" class="col-start-2 row-start-1 flex justify-end md:col-start-5">
						{#if r.original === null}
							<IconButton small label={t.common.remove} onclick={() => toggleDelete(r)}>
								<Trash2 />
							</IconButton>
						{:else}
							<DropdownMenu items={menuFor(r)}>
								{#snippet trigger({ props })}
									<button
										{...props}
										type="button"
										aria-label="{t.common.actions}: {r.key}"
										class="grid size-7 place-items-center rounded-md text-muted-foreground outline-none hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
									>
										<Ellipsis class="size-4" />
									</button>
								{/snippet}
							</DropdownMenu>
						{/if}
					</div>
					<!-- value -->
					<div
						role="cell"
						class="col-span-2 flex min-w-0 items-start gap-1 md:col-span-1 md:col-start-2 md:row-start-1"
					>
						{#if editing === r.id || (r.original === null && canWrite)}
							<Textarea
								bind:value={r.value}
								data-value-editor={r.id}
								aria-label="{t.secrets.value}: {r.key}"
								placeholder={t.secrets.valuePlaceholder}
								rows={1}
								spellcheck={false}
								autocomplete="off"
								class="min-h-8 py-1.5 font-mono text-[13px]"
								onblur={() => stopEdit(r)}
								onkeydown={(e) => {
									if (e.key === 'Escape') (e.currentTarget as HTMLElement).blur();
								}}
							/>
						{:else}
							<button
								type="button"
								disabled={!canWrite || st === 'deleted'}
								onclick={() => startEdit(r)}
								title={canWrite ? t.secrets.editValue : undefined}
								class={cn(
									'min-h-8 min-w-0 flex-1 rounded-md px-2 py-1.5 text-left font-mono text-[13px] break-all whitespace-pre-wrap enabled:cursor-text enabled:hover:bg-muted/70 disabled:cursor-default',
									!isOpen && 'tracking-widest text-muted-foreground select-none'
								)}
								data-testid="secret-value"
							>
								{#if isOpen}{r.value}{:else}••••••••{/if}
							</button>
							{#if r.original !== null}
								<IconButton
									small
									label={revealed.has(r.id) ? t.secrets.hide : t.secrets.reveal}
									onclick={() => (revealed.has(r.id) ? hide(r) : reveal(r))}
								>
									{#if revealed.has(r.id)}<EyeOff />{:else}<Eye />{/if}
								</IconButton>
								<IconButton small label={t.secrets.copyValue} onclick={() => copy(r)}>
									<Copy />
								</IconButton>
							{/if}
						{/if}
					</div>
					<!-- comment -->
					<div role="cell" class="hidden items-center md:flex md:min-h-8">
						{#if r.comment || (r.comment === undefined && r.original?.hasComment)}
							<Tooltip text={r.comment || t.secrets.hasComment}>
								{#snippet trigger({ props })}
									<button
										{...props}
										type="button"
										aria-label={t.secrets.hasComment}
										class="grid size-7 place-items-center rounded-md text-muted-foreground hover:bg-muted"
										onclick={() => (canWrite ? editComment(r) : reveal(r))}
									>
										<MessageSquare class="size-4" />
									</button>
								{/snippet}
							</Tooltip>
						{/if}
					</div>
					<!-- updated -->
					<div
						role="cell"
						class="col-span-2 text-xs text-muted-foreground md:col-span-1 md:flex md:min-h-8 md:items-center"
					>
						{#if r.original}
							<span title="{absoluteTime(r.original.updatedAt)} · {r.original.updatedBy.label}">
								{relativeTime(r.original.updatedAt)}
							</span>
							<span class="md:hidden"> · v{r.original.version}</span>
						{/if}
					</div>
					{#if err}
						<p class="col-span-2 text-xs text-destructive md:col-span-5" role="alert">{err}</p>
					{/if}
				</div>
			{:else}
				<p class="px-3 py-8 text-center text-[13px] text-muted-foreground">{t.secrets.noMatch}</p>
			{/each}
		</div>
		<p class="text-xs text-muted-foreground">{t.secrets.hidden}</p>
	{/if}
</div>

{#if pending > 0}
	<div
		class="fixed inset-x-0 bottom-0 z-40 border-t bg-background/95 px-4 py-3 backdrop-blur lg:left-60"
		data-testid="save-bar"
	>
		<div class="mx-auto flex max-w-6xl items-center gap-3">
			<span class="size-2 rounded-full bg-warning"></span>
			<p class="mr-auto text-[13px] font-medium">{t.secrets.pending(pending)}</p>
			<Button variant="outline" size="sm" onclick={discard} disabled={saving}>
				{t.secrets.discard}
			</Button>
			<Button size="sm" onclick={save} disabled={saving}>
				{saving ? t.common.saving : t.common.save}
			</Button>
		</div>
	</div>
{/if}

<ImportDialog
	bind:open={importOpen}
	{project}
	{env}
	current={currentValues}
	onimported={() => reload()}
/>

<HistorySheet
	bind:open={historyOpen}
	{project}
	{env}
	secretKey={historyKey}
	canWrite={canHistoryValues}
	onrestored={() => reload(new Set([historyKey]))}
/>

<Dialog bind:open={commentOpen} title="{t.secrets.comment}: {commentRow?.key ?? ''}">
	<form id="comment-form" onsubmit={applyComment}>
		<Textarea
			bind:value={commentDraft}
			aria-label={t.secrets.comment}
			placeholder={t.secrets.commentPlaceholder}
			rows={3}
			maxlength={1024}
		/>
	</form>
	{#snippet footer()}
		<Button variant="outline" onclick={() => (commentOpen = false)}>{t.common.cancel}</Button>
		<Button type="submit" form="comment-form">{t.common.save}</Button>
	{/snippet}
</Dialog>

<Dialog
	bind:open={conflictOpen}
	title={t.secrets.conflictTitle}
	description={t.secrets.conflictIntro}
>
	<ul class="grid gap-1.5" data-testid="conflicts">
		{#each Object.entries(conflicts) as [key, why] (key)}
			<li class="rounded-md border px-3 py-2">
				<p class="font-mono text-[13px] font-medium">{key}</p>
				<p class="text-xs text-muted-foreground">{why}</p>
			</li>
		{/each}
	</ul>
	{#snippet footer()}
		<Button variant="outline" onclick={() => (conflictOpen = false)}>{t.common.close}</Button>
		<Button onclick={reloadAfterConflict}>{t.secrets.conflictReload}</Button>
	{/snippet}
</Dialog>
