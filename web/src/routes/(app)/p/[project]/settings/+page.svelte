<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import EnvironmentSettings from '#lib/components/settings/EnvironmentSettings.svelte';
	import GeneralSettings from '#lib/components/settings/GeneralSettings.svelte';
	import MemberSettings from '#lib/components/settings/MemberSettings.svelte';
	import TokenSettings from '#lib/components/settings/TokenSettings.svelte';
	import Tabs from '#lib/components/ui/Tabs.svelte';
	import { t } from '#lib/i18n/index.js';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const isOwner = $derived(data.project.role === 'owner');
	const tabs = $derived([
		{ value: 'general', label: t.settings.general },
		{ value: 'environments', label: t.settings.environments },
		{ value: 'members', label: t.settings.members },
		...(isOwner ? [{ value: 'tokens', label: t.settings.tokens }] : [])
	]);
	// The active tab lives in the URL (?tab=) so it survives reloads and links.
	const tab = $derived(page.url.searchParams.get('tab') ?? 'general');

	function select(v: string) {
		const url = new URL(page.url.href);
		url.searchParams.set('tab', v);
		void goto(url, { replace: true, reset: false });
	}
</script>

<svelte:head><title>{t.settings.title} · {data.project.name} · {t.app.name}</title></svelte:head>

<h1 class="mb-1 text-xl font-semibold tracking-tight">{t.settings.title}</h1>
{#if !isOwner}
	<p class="mb-4 text-[13px] text-muted-foreground">{t.settings.ownerOnly}</p>
{/if}

<div class="mt-4">
	<Tabs value={tab} {tabs} onchange={select}>
		{#snippet content(value)}
			{#key data.project.slug}
				{#if value === 'general'}
					<GeneralSettings project={data.project} {isOwner} />
				{:else if value === 'environments'}
					<EnvironmentSettings project={data.project} {isOwner} />
				{:else if value === 'members'}
					<MemberSettings project={data.project} {isOwner} />
				{:else if value === 'tokens' && isOwner}
					<TokenSettings project={data.project} />
				{/if}
			{/key}
		{/snippet}
	</Tabs>
</div>
