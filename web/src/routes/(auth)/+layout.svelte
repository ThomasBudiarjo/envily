<script lang="ts">
	import KeyRound from '@lucide/svelte/icons/key-round';
	import { t } from '#lib/i18n/index.js';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();
</script>

<main class="grid min-h-dvh place-items-center bg-muted/40 px-4 py-10">
	<div class="w-full max-w-sm">
		<div class="mb-6 flex items-center justify-center gap-2">
			<div class="grid size-8 place-items-center rounded-lg bg-accent text-accent-foreground">
				<KeyRound class="size-4" />
			</div>
			<span class="text-lg font-semibold tracking-tight">{t.app.name}</span>
		</div>
		<div class="rounded-xl border bg-card p-6 shadow-xs">
			{@render children()}
		</div>
	</div>
</main>
