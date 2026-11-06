<script lang="ts">
	import type { HTMLTextareaAttributes } from 'svelte/elements';
	import { cn } from '#lib/utils.js';

	// Grows with its content (field-sizing where supported, JS fallback).
	let {
		value = $bindable(''),
		ref = $bindable(null),
		invalid = false,
		autosize = true,
		class: className,
		...rest
	}: HTMLTextareaAttributes & {
		value?: string;
		ref?: HTMLTextAreaElement | null;
		invalid?: boolean;
		autosize?: boolean;
	} = $props();

	$effect(() => {
		void value;
		if (!autosize || !ref || CSS.supports('field-sizing', 'content')) return;
		ref.style.height = 'auto';
		ref.style.height = ref.scrollHeight + 2 + 'px';
	});
</script>

<textarea
	bind:this={ref}
	bind:value
	aria-invalid={invalid || undefined}
	class={cn(
		'w-full min-w-0 rounded-md border border-input bg-background px-3 py-2 text-sm transition-colors outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/30 disabled:opacity-50 aria-invalid:border-destructive',
		autosize && '[field-sizing:content] resize-none',
		className as string
	)}
	{...rest}></textarea>
