<script lang="ts">
	import { Tooltip } from 'bits-ui';
	import type { Snippet } from 'svelte';

	// Wraps a trigger rendered through `child` so the tooltip attaches to the
	// caller's own element (usually an IconButton).
	let {
		text,
		side = 'top',
		trigger
	}: {
		text: string;
		side?: 'top' | 'bottom' | 'left' | 'right';
		trigger: Snippet<[{ props: Record<string, unknown> }]>;
	} = $props();
</script>

<Tooltip.Root delayDuration={300}>
	<Tooltip.Trigger>
		{#snippet child({ props })}
			{@render trigger({ props })}
		{/snippet}
	</Tooltip.Trigger>
	<Tooltip.Portal>
		<Tooltip.Content
			{side}
			sideOffset={6}
			class="z-50 rounded-md bg-foreground px-2 py-1 text-xs text-background shadow-sm"
		>
			{text}
		</Tooltip.Content>
	</Tooltip.Portal>
</Tooltip.Root>
