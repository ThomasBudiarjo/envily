<script lang="ts">
	import { page } from '$app/state';
	import CircleAlert from '@lucide/svelte/icons/circle-alert';
	import Button from '#lib/components/ui/Button.svelte';
	import EmptyState from '#lib/components/ui/EmptyState.svelte';
	import { t } from '#lib/i18n/index.js';
</script>

<EmptyState
	icon={CircleAlert}
	title={page.status === 404 ? t.common.notFound : t.common.error}
	description={page.error?.message}
>
	<Button variant="outline" href="/">{t.nav.projects}</Button>
</EmptyState>
