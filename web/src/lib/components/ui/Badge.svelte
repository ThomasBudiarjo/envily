<script lang="ts">
	import type { Snippet } from 'svelte';
	import { cn } from '#lib/utils.js';

	const tones = {
		neutral: 'bg-muted text-muted-foreground',
		accent: 'bg-accent/12 text-accent',
		success: 'bg-success/12 text-success',
		warning: 'bg-warning/15 text-warning',
		destructive: 'bg-destructive/12 text-destructive'
	};

	let {
		tone = 'neutral',
		class: className,
		children
	}: { tone?: keyof typeof tones; class?: string; children: Snippet } = $props();
</script>

<span
	class={cn(
		'inline-flex h-5 items-center rounded px-1.5 text-[11px] font-medium whitespace-nowrap',
		tones[tone],
		className
	)}
>
	{@render children()}
</span>
