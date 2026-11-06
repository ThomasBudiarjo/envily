<script lang="ts">
	import { Select } from 'bits-ui';
	import Check from '@lucide/svelte/icons/check';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import { cn } from '#lib/utils.js';

	let {
		value = $bindable(''),
		items,
		id,
		label,
		disabled = false,
		class: className,
		onchange
	}: {
		value?: string;
		items: { value: string; label: string }[];
		id?: string;
		label?: string;
		disabled?: boolean;
		class?: string;
		onchange?: (value: string) => void;
	} = $props();

	const selected = $derived(items.find((i) => i.value === value)?.label ?? '');
</script>

<Select.Root type="single" bind:value {items} {disabled} onValueChange={(v) => onchange?.(v)}>
	<Select.Trigger
		{id}
		aria-label={label}
		class={cn(
			'inline-flex h-9 w-full items-center justify-between gap-2 rounded-md border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring/30 disabled:opacity-50',
			className
		)}
	>
		<span class="truncate">{selected}</span>
		<ChevronDown class="size-4 text-muted-foreground" />
	</Select.Trigger>
	<Select.Portal>
		<Select.Content
			sideOffset={4}
			class="z-50 max-h-(--bits-select-content-available-height) min-w-(--bits-select-anchor-width) rounded-lg border bg-popover p-1 shadow-md outline-none"
		>
			<Select.Viewport>
				{#each items as item (item.value)}
					<Select.Item
						value={item.value}
						label={item.label}
						class="flex h-8 cursor-pointer items-center justify-between gap-4 rounded-md px-2 text-[13px] outline-none select-none data-highlighted:bg-muted"
					>
						{#snippet children({ selected })}
							{item.label}
							{#if selected}<Check class="size-4" />{/if}
						{/snippet}
					</Select.Item>
				{/each}
			</Select.Viewport>
		</Select.Content>
	</Select.Portal>
</Select.Root>
