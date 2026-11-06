<script lang="ts">
	import { Checkbox } from 'bits-ui';
	import Check from '@lucide/svelte/icons/check';

	let {
		checked = $bindable(false),
		id,
		label,
		disabled = false
	}: { checked?: boolean; id?: string; label?: string; disabled?: boolean } = $props();
</script>

<Checkbox.Root
	bind:checked
	{id}
	{disabled}
	aria-label={label}
	class="inline-flex size-4 shrink-0 cursor-pointer items-center justify-center rounded border border-input bg-background text-accent-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 data-[state=checked]:border-accent data-[state=checked]:bg-accent"
>
	{#snippet children({ checked })}
		{#if checked}<Check class="size-3" strokeWidth={3} />{/if}
	{/snippet}
</Checkbox.Root>
