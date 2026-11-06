<script lang="ts">
	import Eye from '@lucide/svelte/icons/eye';
	import EyeOff from '@lucide/svelte/icons/eye-off';
	import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
	import { untrack } from 'svelte';
	import { toast } from 'svelte-sonner';
	import { SvelteSet } from 'svelte/reactivity';
	import { listVersions, rollbackSecret } from '#lib/api/secrets.js';
	import type { SecretVersion } from '#lib/api/types.js';
	import Badge from '#lib/components/ui/Badge.svelte';
	import Button from '#lib/components/ui/Button.svelte';
	import ConfirmDialog from '#lib/components/ui/ConfirmDialog.svelte';
	import IconButton from '#lib/components/ui/IconButton.svelte';
	import Sheet from '#lib/components/ui/Sheet.svelte';
	import { errorMessage } from '#lib/forms.js';
	import { t } from '#lib/i18n/index.js';
	import { absoluteTime, relativeTime } from '#lib/utils.js';

	// Version history of one secret. Editors and owners may see old values
	// and restore a version; viewers see the list only.
	let {
		open = $bindable(false),
		project,
		env,
		secretKey,
		canWrite,
		onrestored
	}: {
		open?: boolean;
		project: string;
		env: string;
		secretKey: string;
		canWrite: boolean;
		onrestored: () => void;
	} = $props();

	let versions = $state<SecretVersion[]>([]);
	let loading = $state(false);
	let withValues = $state(false);
	const shown = new SvelteSet<number>();
	let restoring = $state<number | null>(null);
	let confirmOpen = $state(false);

	async function load(values: boolean) {
		loading = true;
		try {
			versions = await listVersions(project, env, secretKey, values);
			withValues = values;
		} catch (e) {
			toast.error(errorMessage(e));
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		if (open && secretKey) {
			untrack(() => {
				shown.clear();
				withValues = false;
				void load(false);
			});
		} else {
			versions = [];
		}
	});

	async function toggle(v: number) {
		if (shown.has(v)) {
			shown.delete(v);
			return;
		}
		if (!withValues) await load(true);
		shown.add(v);
	}

	async function restore() {
		if (restoring === null) return;
		try {
			const r = await rollbackSecret(project, env, secretKey, restoring);
			toast.success(t.history.rolledBack(r.version));
			onrestored();
			shown.clear();
			await load(false);
		} catch (e) {
			toast.error(errorMessage(e));
		}
	}

	const latest = $derived(versions[0]?.version);
</script>

<Sheet bind:open title={t.history.title(secretKey)}>
	{#if loading && versions.length === 0}
		<p class="p-4 text-[13px] text-muted-foreground">{t.common.loading}</p>
	{:else if versions.length === 0}
		<p class="p-4 text-[13px] text-muted-foreground">{t.history.empty}</p>
	{:else}
		<ol class="divide-y">
			{#each versions as v (v.version)}
				<li class="grid gap-2 px-4 py-3">
					<div class="flex items-center gap-2">
						<span class="text-[13px] font-medium">{t.history.version(v.version)}</span>
						{#if v.version === latest}<Badge tone="accent">{t.history.current}</Badge>{/if}
						<div class="ml-auto flex items-center gap-0.5">
							{#if canWrite}
								<IconButton
									small
									label={shown.has(v.version) ? t.secrets.hide : t.secrets.reveal}
									onclick={() => toggle(v.version)}
								>
									{#if shown.has(v.version)}<EyeOff />{:else}<Eye />{/if}
								</IconButton>
								{#if v.version !== latest}
									<IconButton
										small
										label={t.history.rollback}
										onclick={() => {
											restoring = v.version;
											confirmOpen = true;
										}}
									>
										<RotateCcw />
									</IconButton>
								{/if}
							{/if}
						</div>
					</div>
					<p class="text-xs text-muted-foreground">
						<span title={absoluteTime(v.createdAt)}>{relativeTime(v.createdAt)}</span>
						· {t.history.by(v.actor.label || v.actor.type)}
					</p>
					{#if shown.has(v.version)}
						<pre
							class="max-h-48 overflow-auto rounded-md border bg-muted/40 p-2 font-mono text-xs break-all whitespace-pre-wrap">{v.value}</pre>
						{#if v.comment}<p class="text-xs text-muted-foreground">{v.comment}</p>{/if}
					{/if}
				</li>
			{/each}
		</ol>
	{/if}
	{#if !canWrite}
		<div class="p-4">
			<Button variant="outline" size="sm" onclick={() => (open = false)}>{t.common.close}</Button>
		</div>
	{/if}
</Sheet>

<ConfirmDialog
	bind:open={confirmOpen}
	title={t.history.rollbackTitle(restoring ?? 0)}
	description={t.history.rollbackIntro}
	confirmLabel={t.history.rollback}
	destructive={false}
	onconfirm={restore}
/>
