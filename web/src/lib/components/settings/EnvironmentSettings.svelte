<script lang="ts">
	import { goto, invalidate } from '$app/navigation';
	import { page } from '$app/state';
	import ArrowDown from '@lucide/svelte/icons/arrow-down';
	import ArrowUp from '@lucide/svelte/icons/arrow-up';
	import Plus from '@lucide/svelte/icons/plus';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import { toast } from 'svelte-sonner';
	import { createEnvironment, deleteEnvironment, updateEnvironment } from '#lib/api/projects.js';
	import type { Environment, Project } from '#lib/api/types.js';
	import Button from '#lib/components/ui/Button.svelte';
	import ConfirmDialog from '#lib/components/ui/ConfirmDialog.svelte';
	import IconButton from '#lib/components/ui/IconButton.svelte';
	import Input from '#lib/components/ui/Input.svelte';
	import { errorMessage } from '#lib/forms.js';
	import { t } from '#lib/i18n/index.js';

	let { project, isOwner }: { project: Project; isOwner: boolean } = $props();

	let newName = $state('');
	let busy = $state(false);
	let toDelete = $state<Environment | null>(null);
	let deleteOpen = $state(false);

	async function refresh() {
		await Promise.all([invalidate('app:project'), invalidate('app:projects')]);
	}

	async function add(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		try {
			await createEnvironment(project.slug, newName.trim());
			newName = '';
			toast.success(t.settings.envCreated);
			await refresh();
		} catch (err) {
			toast.error(errorMessage(err));
		} finally {
			busy = false;
		}
	}

	async function rename(env: Environment, field: 'name' | 'slug', value: string) {
		value = value.trim();
		if (!value || value === env[field]) return;
		try {
			await updateEnvironment(project.slug, env.slug, { [field]: value });
			await refresh();
		} catch (err) {
			toast.error(errorMessage(err));
			await refresh();
		}
	}

	// Positions are plain numbers; renumber the list and patch what moved.
	async function move(index: number, delta: number) {
		const order = [...project.environments];
		const [env] = order.splice(index, 1);
		order.splice(index + delta, 0, env);
		try {
			for (const [i, e] of order.entries()) {
				if (e.position !== i) await updateEnvironment(project.slug, e.slug, { position: i });
			}
		} catch (err) {
			toast.error(errorMessage(err));
		}
		await refresh();
	}

	async function remove() {
		if (!toDelete) return;
		const slug = toDelete.slug;
		try {
			await deleteEnvironment(project.slug, slug);
			if (page.url.pathname.includes(`/${slug}`)) await goto(`/p/${project.slug}/settings`);
			await refresh();
		} catch (err) {
			toast.error(errorMessage(err));
			throw err;
		}
	}
</script>

<div class="grid max-w-2xl gap-4">
	<ul class="divide-y rounded-lg border bg-card" data-testid="environment-list">
		{#each project.environments as env, i (env.id)}
			<li class="flex flex-wrap items-center gap-2 px-3 py-2.5 sm:flex-nowrap">
				{#if isOwner}
					<Input
						value={env.name}
						aria-label="{t.common.name}: {env.slug}"
						class="h-8 sm:w-56"
						onchange={(e) => rename(env, 'name', e.currentTarget.value)}
					/>
					<Input
						value={env.slug}
						aria-label="{t.common.slug}: {env.slug}"
						class="h-8 font-mono text-xs sm:w-44"
						onchange={(e) => rename(env, 'slug', e.currentTarget.value)}
					/>
					<div class="ml-auto flex items-center">
						<IconButton
							small
							label={t.common.moveUp}
							disabled={i === 0}
							onclick={() => move(i, -1)}
						>
							<ArrowUp />
						</IconButton>
						<IconButton
							small
							label={t.common.moveDown}
							disabled={i === project.environments.length - 1}
							onclick={() => move(i, 1)}
						>
							<ArrowDown />
						</IconButton>
						<IconButton
							small
							label="{t.settings.deleteEnvironment}: {env.name}"
							class="text-destructive"
							onclick={() => {
								toDelete = env;
								deleteOpen = true;
							}}
						>
							<Trash2 />
						</IconButton>
					</div>
				{:else}
					<span class="font-medium">{env.name}</span>
					<span class="font-mono text-xs text-muted-foreground">{env.slug}</span>
				{/if}
			</li>
		{/each}
	</ul>
	{#if isOwner}
		<form class="flex gap-2" onsubmit={add}>
			<Input
				bind:value={newName}
				placeholder={t.common.name}
				aria-label={t.settings.addEnvironment}
				maxlength={64}
				class="max-w-xs"
			/>
			<Button type="submit" variant="outline" disabled={busy || !newName.trim()}>
				<Plus />{t.settings.addEnvironment}
			</Button>
		</form>
	{/if}
</div>

<ConfirmDialog
	bind:open={deleteOpen}
	title={t.settings.deleteEnvironmentTitle(toDelete?.name ?? '')}
	description={t.settings.deleteEnvironmentIntro}
	confirmText={toDelete?.slug}
	confirmLabel={t.settings.deleteEnvironment}
	onconfirm={remove}
/>
