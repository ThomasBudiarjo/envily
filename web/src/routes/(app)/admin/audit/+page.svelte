<script lang="ts">
	import { instanceAudit } from '#lib/api/audit.js';
	import AuditTable from '#lib/components/AuditTable.svelte';
	import { t } from '#lib/i18n/index.js';
</script>

<svelte:head><title>{t.audit.instanceTitle} · {t.app.name}</title></svelte:head>

<h1 class="mb-5 text-xl font-semibold tracking-tight">{t.audit.instanceTitle}</h1>
<AuditTable fetchPage={instanceAudit} />
