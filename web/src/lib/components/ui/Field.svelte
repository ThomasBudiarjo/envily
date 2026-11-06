<script lang="ts">
	import type { Snippet } from 'svelte';
	import Label from './Label.svelte';

	// Label + control + hint/error, with ids wired for accessibility.
	let {
		id,
		label,
		hint,
		error,
		children
	}: { id: string; label: string; hint?: string; error?: string; children: Snippet } = $props();
</script>

<div class="grid gap-1.5">
	<Label for={id}>{label}</Label>
	{@render children()}
	{#if error}
		<p id="{id}-error" class="text-xs text-destructive">{error}</p>
	{:else if hint}
		<p id="{id}-hint" class="text-xs text-muted-foreground">{hint}</p>
	{/if}
</div>
