<script lang="ts">
	import { projectAudit } from '#lib/api/audit.js';
	import AuditTable from '#lib/components/AuditTable.svelte';
	import { t } from '#lib/i18n/index.js';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head><title>{t.audit.title} · {data.project.name} · {t.app.name}</title></svelte:head>

<h1 class="mb-5 text-xl font-semibold tracking-tight">{t.audit.title}</h1>
{#key data.project.slug}
	<AuditTable fetchPage={(f) => projectAudit(data.project.slug, f)} />
{/key}
