<script lang="ts">
	import { Switch } from 'bits-ui';

	let {
		checked = $bindable(false),
		id,
		label,
		disabled = false,
		onchange
	}: {
		checked?: boolean;
		id?: string;
		label?: string;
		disabled?: boolean;
		onchange?: (checked: boolean) => void;
	} = $props();
</script>

<Switch.Root
	bind:checked
	{id}
	{disabled}
	aria-label={label}
	onCheckedChange={(v) => onchange?.(v)}
	class="inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent bg-input transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 data-[state=checked]:bg-accent"
>
	<Switch.Thumb
		class="pointer-events-none block size-4 rounded-full bg-background shadow-sm transition-transform data-[state=checked]:translate-x-4"
	/>
</Switch.Root>
