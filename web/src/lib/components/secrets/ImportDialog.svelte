<script lang="ts">
	import Upload from '@lucide/svelte/icons/upload';
	import { toast } from 'svelte-sonner';
	import { importSecrets } from '#lib/api/secrets.js';
	import type { ImportMode } from '#lib/api/types.js';
	import Button from '#lib/components/ui/Button.svelte';
	import Dialog from '#lib/components/ui/Dialog.svelte';
	import Label from '#lib/components/ui/Label.svelte';
	import Select from '#lib/components/ui/Select.svelte';
	import Textarea from '#lib/components/ui/Textarea.svelte';
	import { parseImport, previewImport } from '#lib/dotenv.js';
	import { errorMessage } from '#lib/forms.js';
	import { t } from '#lib/i18n/index.js';

	let {
		open = $bindable(false),
		project,
		env,
		current,
		onimported
	}: {
		open?: boolean;
		project: string;
		env: string;
		/** Live keys and, when loaded, their values. */
		current: Map<string, string | undefined>;
		onimported: () => void;
	} = $props();

	let text = $state('');
	let mode = $state<ImportMode>('merge');
	let busy = $state(false);
	let fileInput = $state<HTMLInputElement | null>(null);

	$effect(() => {
		if (open) {
			text = '';
			mode = 'merge';
		}
	});

	const parsed = $derived.by(() => {
		if (!text.trim()) return { preview: null, error: '' };
		try {
			return { preview: previewImport(parseImport(text), current, mode), error: '' };
		} catch (e) {
			return { preview: null, error: (e as Error).message };
		}
	});

	async function pick(e: Event & { currentTarget: HTMLInputElement }) {
		const file = e.currentTarget.files?.[0];
		if (file) text = await file.text();
		e.currentTarget.value = '';
	}

	async function submit() {
		busy = true;
		try {
			const r = await importSecrets(project, env, text, mode);
			toast.success(t.importDialog.done(r.created, r.updated, r.deleted));
			text = '';
			open = false;
			onimported();
		} catch (e) {
			toast.error(errorMessage(e));
		} finally {
			busy = false;
		}
	}
</script>

<Dialog bind:open title={t.importDialog.title} description={t.importDialog.intro} class="max-w-2xl">
	<div class="grid gap-4">
		<div class="grid gap-1.5">
			<div class="flex items-center justify-between">
				<Label for="import-text">.env / JSON</Label>
				<Button variant="ghost" size="sm" onclick={() => fileInput?.click()}>
					<Upload />{t.importDialog.chooseFile}
				</Button>
				<input
					bind:this={fileInput}
					type="file"
					accept=".env,.json,.txt,text/plain,application/json"
					class="hidden"
					onchange={pick}
				/>
			</div>
			<Textarea
				id="import-text"
				bind:value={text}
				autosize={false}
				spellcheck={false}
				autocomplete="off"
				placeholder="DATABASE_URL=postgres://…&#10;API_KEY=…"
				class="h-48 resize-y font-mono text-xs"
				invalid={!!parsed.error}
			/>
			{#if parsed.error}<p class="text-xs text-destructive">{parsed.error}</p>{/if}
		</div>
		<div class="grid gap-1.5">
			<Label for="import-mode">{t.importDialog.mode}</Label>
			<Select
				id="import-mode"
				bind:value={mode}
				items={[
					{ value: 'merge', label: t.importDialog.merge },
					{ value: 'overwrite', label: t.importDialog.overwrite }
				]}
			/>
		</div>
		<div class="grid gap-2 rounded-lg border p-3" data-testid="import-preview">
			<p class="text-[13px] font-medium">{t.importDialog.preview}</p>
			{#if !parsed.preview}
				<p class="text-[13px] text-muted-foreground">{t.importDialog.nothing}</p>
			{:else}
				{#each [{ label: t.importDialog.added, keys: parsed.preview.added, tone: 'text-success' }, { label: t.importDialog.changed, keys: parsed.preview.changed, tone: 'text-warning' }, { label: t.importDialog.removed, keys: parsed.preview.removed, tone: 'text-destructive' }, { label: t.importDialog.invalid, keys: parsed.preview.invalid, tone: 'text-destructive' }] as group (group.label)}
					{#if group.keys.length}
						<div class="grid gap-1">
							<p class="text-xs font-medium {group.tone}">{group.label} ({group.keys.length})</p>
							<p class="font-mono text-xs break-all text-muted-foreground">
								{group.keys.join(', ')}
							</p>
						</div>
					{/if}
				{/each}
			{/if}
		</div>
	</div>
	{#snippet footer()}
		<Button variant="outline" onclick={() => (open = false)}>{t.common.cancel}</Button>
		<Button
			onclick={submit}
			disabled={busy || !parsed.preview || parsed.preview.invalid.length > 0}
		>
			{busy ? t.common.saving : t.importDialog.submit}
		</Button>
	{/snippet}
</Dialog>
