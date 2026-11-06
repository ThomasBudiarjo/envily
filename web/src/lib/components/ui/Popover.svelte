<script lang="ts">
	import { Popover } from 'bits-ui';
	import type { Snippet } from 'svelte';

	let {
		open = $bindable(false),
		trigger,
		children,
		align = 'start'
	}: {
		open?: boolean;
		trigger: Snippet<[{ props: Record<string, unknown> }]>;
		children: Snippet;
		align?: 'start' | 'center' | 'end';
	} = $props();
</script>

<Popover.Root bind:open>
	<Popover.Trigger>
		{#snippet child({ props })}
			{@render trigger({ props })}
		{/snippet}
	</Popover.Trigger>
	<Popover.Portal>
		<Popover.Content
			{align}
			sideOffset={6}
			class="z-50 w-72 rounded-lg border bg-popover p-3 shadow-md outline-none"
		>
			{@render children()}
		</Popover.Content>
	</Popover.Portal>
</Popover.Root>
