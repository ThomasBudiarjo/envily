<script lang="ts">
	import { page } from '$app/state';
	import CommandPalette from '#lib/components/app/CommandPalette.svelte';
	import Header from '#lib/components/app/Header.svelte';
	import Sidebar from '#lib/components/app/Sidebar.svelte';
	import Sheet from '#lib/components/ui/Sheet.svelte';
	import { t } from '#lib/i18n/index.js';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();

	let navOpen = $state(false);
	let paletteOpen = $state(false);

	// Close the mobile drawer after every navigation.
	$effect(() => {
		void page.url.pathname;
		navOpen = false;
	});

	function onkeydown(e: KeyboardEvent) {
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
			e.preventDefault();
			paletteOpen = !paletteOpen;
		}
	}
</script>

<svelte:window {onkeydown} />

<div class="flex min-h-dvh">
	<aside class="sticky top-0 hidden h-dvh w-60 shrink-0 border-r bg-muted/30 lg:block">
		<Sidebar projects={data.projects} />
	</aside>
	<Sheet bind:open={navOpen} title={t.app.name} side="left" class="w-72">
		<Sidebar projects={data.projects} brand={false} />
	</Sheet>
	<div class="flex min-w-0 flex-1 flex-col">
		<Header onmenu={() => (navOpen = true)} onpalette={() => (paletteOpen = true)} />
		<main class="mx-auto w-full max-w-6xl flex-1 px-4 py-6 sm:px-6 lg:px-8">
			{@render children()}
		</main>
	</div>
</div>

<CommandPalette bind:open={paletteOpen} projects={data.projects} />
