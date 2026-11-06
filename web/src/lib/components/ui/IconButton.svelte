<script lang="ts">
	import type { HTMLButtonAttributes } from 'svelte/elements';
	import { buttonClass, type ButtonVariant } from './Button.svelte';
	import Tooltip from './Tooltip.svelte';
	import { cn } from '#lib/utils.js';

	// An icon-only button. `label` is both the accessible name and the tooltip.
	let {
		label,
		variant = 'ghost',
		small = false,
		class: className,
		children,
		...rest
	}: HTMLButtonAttributes & { label: string; variant?: ButtonVariant; small?: boolean } = $props();
</script>

<Tooltip text={label}>
	{#snippet trigger({ props })}
		<button
			type="button"
			aria-label={label}
			{...props}
			{...rest}
			class={cn(buttonClass(variant, small ? 'icon-sm' : 'icon'), className as string)}
		>
			{@render children?.()}
		</button>
	{/snippet}
</Tooltip>
