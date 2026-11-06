<script lang="ts">
	import { Combobox } from 'bits-ui';
	import Check from '@lucide/svelte/icons/check';
	import ChevronsUpDown from '@lucide/svelte/icons/chevrons-up-down';

	// Single-select search box. Filtering is done here on label and hint.
	let {
		value = $bindable(''),
		items,
		id,
		placeholder,
		empty
	}: {
		value?: string;
		items: { value: string; label: string; hint?: string }[];
		id?: string;
		placeholder?: string;
		empty: string;
	} = $props();

	let search = $state('');
	const filtered = $derived.by(() => {
		const q = search.trim().toLowerCase();
		if (!q) return items;
		return items.filter(
			(i) => i.label.toLowerCase().includes(q) || i.hint?.toLowerCase().includes(q)
		);
	});
</script>

<Combobox.Root
	type="single"
	bind:value
	items={items.map((i) => ({ value: i.value, label: i.label }))}
	onOpenChangeComplete={(o) => {
		if (!o) search = '';
	}}
>
	<div class="relative">
		<Combobox.Input
			{id}
			{placeholder}
			oninput={(e) => (search = e.currentTarget.value)}
			class="h-9 w-full rounded-md border border-input bg-background pr-9 pl-3 text-sm outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/30"
		/>
		<Combobox.Trigger
			class="absolute top-1/2 right-2 -translate-y-1/2 rounded p-1 text-muted-foreground"
		>
			<ChevronsUpDown class="size-4" />
		</Combobox.Trigger>
	</div>
	<Combobox.Portal>
		<Combobox.Content
			sideOffset={4}
			class="z-50 max-h-64 w-(--bits-combobox-anchor-width) overflow-y-auto rounded-lg border bg-popover p-1 shadow-md outline-none"
		>
			{#each filtered as item (item.value)}
				<Combobox.Item
					value={item.value}
					label={item.label}
					class="flex cursor-pointer items-center justify-between gap-3 rounded-md px-2 py-1.5 text-[13px] outline-none select-none data-highlighted:bg-muted"
				>
					{#snippet children({ selected })}
						<span class="min-w-0">
							<span class="block truncate">{item.label}</span>
							{#if item.hint}
								<span class="block truncate text-xs text-muted-foreground">{item.hint}</span>
							{/if}
						</span>
						{#if selected}<Check class="size-4 shrink-0" />{/if}
					{/snippet}
				</Combobox.Item>
			{:else}
				<p class="px-2 py-3 text-center text-[13px] text-muted-foreground">{empty}</p>
			{/each}
		</Combobox.Content>
	</Combobox.Portal>
</Combobox.Root>
