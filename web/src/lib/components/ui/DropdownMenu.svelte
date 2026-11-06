<script lang="ts" module>
	import type { Component } from 'svelte';

	export interface MenuItem {
		label: string;
		icon?: Component;
		onselect: () => void;
		destructive?: boolean;
		disabled?: boolean;
		separatorBefore?: boolean;
	}
</script>

<script lang="ts">
	import { DropdownMenu } from 'bits-ui';
	import type { Snippet } from 'svelte';
	import { cn } from '#lib/utils.js';

	let {
		items,
		align = 'end',
		trigger,
		header
	}: {
		items: MenuItem[];
		align?: 'start' | 'center' | 'end';
		trigger: Snippet<[{ props: Record<string, unknown> }]>;
		header?: Snippet;
	} = $props();
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger>
		{#snippet child({ props })}
			{@render trigger({ props })}
		{/snippet}
	</DropdownMenu.Trigger>
	<DropdownMenu.Portal>
		<DropdownMenu.Content
			{align}
			sideOffset={4}
			class="z-50 min-w-44 rounded-lg border bg-popover p-1 shadow-md outline-none"
		>
			{#if header}
				<div class="px-2 py-1.5">{@render header()}</div>
				<DropdownMenu.Separator class="-mx-1 my-1 h-px bg-border" />
			{/if}
			{#each items as item (item.label)}
				{#if item.separatorBefore}
					<DropdownMenu.Separator class="-mx-1 my-1 h-px bg-border" />
				{/if}
				<DropdownMenu.Item
					disabled={item.disabled}
					onSelect={item.onselect}
					class={cn(
						'flex h-8 cursor-pointer items-center gap-2 rounded-md px-2 text-[13px] outline-none select-none data-disabled:pointer-events-none data-disabled:opacity-50 data-highlighted:bg-muted [&_svg]:size-4',
						item.destructive && 'text-destructive'
					)}
				>
					{#if item.icon}<item.icon />{/if}
					{item.label}
				</DropdownMenu.Item>
			{/each}
		</DropdownMenu.Content>
	</DropdownMenu.Portal>
</DropdownMenu.Root>
