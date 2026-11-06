<script lang="ts">
	import { goto, invalidate } from '$app/navigation';
	import { toast } from 'svelte-sonner';
	import { deleteProject, updateProject } from '#lib/api/projects.js';
	import type { Project } from '#lib/api/types.js';
	import Button from '#lib/components/ui/Button.svelte';
	import ConfirmDialog from '#lib/components/ui/ConfirmDialog.svelte';
	import Field from '#lib/components/ui/Field.svelte';
	import Input from '#lib/components/ui/Input.svelte';
	import { errorMessage, fieldErrors } from '#lib/forms.js';
	import { t } from '#lib/i18n/index.js';

	let { project, isOwner }: { project: Project; isOwner: boolean } = $props();

	// svelte-ignore state_referenced_locally
	let name = $state(project.name);
	// svelte-ignore state_referenced_locally
	let slug = $state(project.slug);
	let errors = $state<Record<string, string>>({});
	let busy = $state(false);
	let deleting = $state(false);

	async function save(e: SubmitEvent) {
		e.preventDefault();
		errors = {};
		busy = true;
		try {
			const p = await updateProject(project.slug, {
				name: name.trim() !== project.name ? name.trim() : undefined,
				slug: slug !== project.slug ? slug : undefined
			});
			toast.success(t.settings.saved);
			await invalidate('app:projects');
			if (p.slug !== project.slug) await goto(`/p/${p.slug}/settings`, { replace: true });
			else await invalidate('app:project');
		} catch (err) {
			errors = fieldErrors(err);
			if (!Object.keys(errors).length) toast.error(errorMessage(err));
		} finally {
			busy = false;
		}
	}

	async function remove() {
		try {
			await deleteProject(project.slug);
			toast.success(t.settings.deleted);
			await invalidate('app:projects');
			await goto('/', { replace: true });
		} catch (err) {
			toast.error(errorMessage(err));
			throw err;
		}
	}
</script>

<form class="grid max-w-md gap-4" onsubmit={save}>
	<Field id="settings-name" label={t.settings.projectName} error={errors.name}>
		<Input id="settings-name" bind:value={name} disabled={!isOwner} maxlength={64} required />
	</Field>
	<Field id="settings-slug" label={t.common.slug} hint={t.common.slugHint} error={errors.slug}>
		<Input
			id="settings-slug"
			bind:value={slug}
			disabled={!isOwner}
			class="font-mono"
			invalid={!!errors.slug}
		/>
	</Field>
	{#if isOwner}
		<div>
			<Button
				type="submit"
				disabled={busy || (name.trim() === project.name && slug === project.slug)}
			>
				{busy ? t.common.saving : t.common.save}
			</Button>
		</div>
	{/if}
</form>

{#if isOwner}
	<section class="mt-10 grid max-w-2xl gap-3 rounded-lg border border-destructive/40 p-4">
		<h2 class="text-sm font-semibold text-destructive">{t.settings.dangerZone}</h2>
		<p class="text-[13px] text-muted-foreground">{t.settings.deleteProjectIntro}</p>
		<div>
			<Button variant="destructive" onclick={() => (deleting = true)}
				>{t.settings.deleteProject}</Button
			>
		</div>
	</section>
	<ConfirmDialog
		bind:open={deleting}
		title={t.settings.deleteProjectTitle(project.name)}
		description={t.settings.deleteProjectIntro}
		confirmText={project.slug}
		confirmLabel={t.settings.deleteProject}
		onconfirm={remove}
	/>
{/if}
