<script lang="ts">
	import type { HTMLInputAttributes } from 'svelte/elements';
	import { cn } from '#lib/utils.js';

	let {
		value = $bindable(),
		ref = $bindable(null),
		invalid = false,
		class: className,
		...rest
	}: HTMLInputAttributes & { ref?: HTMLInputElement | null; invalid?: boolean } = $props();
</script>

<input
	bind:this={ref}
	bind:value
	aria-invalid={invalid || undefined}
	class={cn(
		'h-9 w-full min-w-0 rounded-md border border-input bg-background px-3 text-sm transition-colors outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/30 disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20',
		className as string
	)}
	{...rest}
/>
