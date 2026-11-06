<script lang="ts">
	import type { Component, Snippet } from 'svelte';

	let {
		icon: Icon,
		title,
		description,
		children
	}: { icon: Component; title: string; description?: string; children?: Snippet } = $props();
</script>

<div class="grid place-items-center gap-3 rounded-lg border border-dashed px-6 py-14 text-center">
	<div class="grid size-11 place-items-center rounded-full bg-muted text-muted-foreground">
		<Icon class="size-5" />
	</div>
	<div class="grid gap-1">
		<p class="font-medium">{title}</p>
		{#if description}<p class="max-w-sm text-[13px] text-muted-foreground">{description}</p>{/if}
	</div>
	{#if children}<div class="mt-1 flex flex-wrap justify-center gap-2">
			{@render children()}
		</div>{/if}
</div>
