<script lang="ts">
	import { goto, invalidate } from '$app/navigation';
	import { createProject } from '#lib/api/projects.js';
	import Button from '#lib/components/ui/Button.svelte';
	import Dialog from '#lib/components/ui/Dialog.svelte';
	import Field from '#lib/components/ui/Field.svelte';
	import Input from '#lib/components/ui/Input.svelte';
	import { errorMessage, fieldErrors } from '#lib/forms.js';
	import { t } from '#lib/i18n/index.js';
	import { SLUG_RE, slugify } from '#lib/utils.js';

	let { open = $bindable(false) }: { open?: boolean } = $props();

	let name = $state('');
	let slug = $state('');
	let slugTouched = $state(false);
	let errors = $state<Record<string, string>>({});
	let busy = $state(false);
	const effectiveSlug = $derived(slugTouched ? slug : slugify(name));

	$effect(() => {
		if (open) {
			name = slug = '';
			slugTouched = false;
			errors = {};
		}
	});

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errors = {};
		if (!SLUG_RE.test(effectiveSlug)) {
			errors = { slug: t.common.slugHint };
			return;
		}
		busy = true;
		try {
			const p = await createProject(name.trim(), effectiveSlug);
			await invalidate('app:projects');
			open = false;
			await goto(`/p/${p.slug}/${p.environments[0]?.slug ?? ''}`);
		} catch (err) {
			errors = fieldErrors(err);
			if (!Object.keys(errors).length) errors = { name: errorMessage(err) };
		} finally {
			busy = false;
		}
	}
</script>

<Dialog bind:open title={t.projects.createTitle} description={t.projects.createIntro}>
	<form id="create-project" class="grid gap-4" onsubmit={submit}>
		<Field id="project-name" label={t.common.name} error={errors.name}>
			<Input id="project-name" bind:value={name} required maxlength={64} autocomplete="off" />
		</Field>
		<Field id="project-slug" label={t.common.slug} hint={t.common.slugHint} error={errors.slug}>
			<Input
				id="project-slug"
				value={effectiveSlug}
				oninput={(e) => {
					slugTouched = true;
					slug = e.currentTarget.value;
				}}
				class="font-mono"
				autocomplete="off"
				invalid={!!errors.slug}
			/>
		</Field>
	</form>
	{#snippet footer()}
		<Button variant="outline" onclick={() => (open = false)}>{t.common.cancel}</Button>
		<Button type="submit" form="create-project" disabled={busy || !name.trim()}>
			{busy ? t.common.creating : t.common.create}
		</Button>
	{/snippet}
</Dialog>
