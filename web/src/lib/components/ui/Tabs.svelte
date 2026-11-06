<script lang="ts">
	import { Tabs } from 'bits-ui';
	import type { Snippet } from 'svelte';

	let {
		value = $bindable(''),
		tabs,
		content,
		onchange
	}: {
		value?: string;
		tabs: { value: string; label: string }[];
		content: Snippet<[string]>;
		onchange?: (value: string) => void;
	} = $props();
</script>

<Tabs.Root bind:value onValueChange={(v) => onchange?.(v)}>
	<Tabs.List class="-mx-1 flex gap-1 overflow-x-auto border-b px-1">
		{#each tabs as tab (tab.value)}
			<Tabs.Trigger
				value={tab.value}
				class="-mb-px shrink-0 cursor-pointer border-b-2 border-transparent px-3 py-2 text-[13px] font-medium text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring data-[state=active]:border-accent data-[state=active]:text-foreground"
			>
				{tab.label}
			</Tabs.Trigger>
		{/each}
	</Tabs.List>
	{#each tabs as tab (tab.value)}
		<Tabs.Content value={tab.value} class="pt-5 outline-none">
			{@render content(tab.value)}
		</Tabs.Content>
	{/each}
</Tabs.Root>
