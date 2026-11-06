<script lang="ts">
	import SecretsTable from '#lib/components/secrets/SecretsTable.svelte';
	import { t } from '#lib/i18n/index.js';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const role = $derived(data.project.role);
	const canWrite = $derived(role === 'owner' || role === 'editor');
</script>

<svelte:head><title>{data.env.name} · {data.project.name} · {t.app.name}</title></svelte:head>

<!-- Remount per environment so drafts, revealed values, and timers never leak across. -->
{#key `${data.project.slug}/${data.env.slug}`}
	<SecretsTable
		project={data.project.slug}
		env={data.env.slug}
		envName={data.env.name}
		initial={data.secrets}
		{canWrite}
		canHistoryValues={canWrite}
	/>
{/key}
