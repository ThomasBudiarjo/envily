<script lang="ts" module>
	export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'destructive' | 'outline';
	export type ButtonSize = 'sm' | 'md' | 'icon' | 'icon-sm';

	const variants: Record<ButtonVariant, string> = {
		primary: 'bg-accent text-accent-foreground hover:bg-accent/90',
		secondary: 'bg-muted text-foreground hover:bg-muted/70',
		outline: 'border border-border bg-background hover:bg-muted',
		ghost: 'text-foreground hover:bg-muted',
		destructive: 'bg-destructive text-destructive-foreground hover:bg-destructive/90'
	};
	const sizes: Record<ButtonSize, string> = {
		sm: 'h-8 px-3 text-[13px] gap-1.5',
		md: 'h-9 px-3.5 gap-2',
		icon: 'size-9',
		'icon-sm': 'size-7'
	};

	export function buttonClass(variant: ButtonVariant = 'primary', size: ButtonSize = 'md') {
		return `inline-flex shrink-0 cursor-pointer items-center justify-center rounded-md font-medium whitespace-nowrap transition-colors outline-none select-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50 [&_svg]:size-4 [&_svg]:shrink-0 ${variants[variant]} ${sizes[size]}`;
	}
</script>

<script lang="ts">
	import type { HTMLAnchorAttributes, HTMLButtonAttributes } from 'svelte/elements';
	import { cn } from '#lib/utils.js';

	type Props = (HTMLButtonAttributes & HTMLAnchorAttributes) & {
		variant?: ButtonVariant;
		size?: ButtonSize;
		href?: string;
		ref?: HTMLElement | null;
	};

	let {
		variant = 'primary',
		size = 'md',
		href,
		type = 'button',
		class: className,
		ref = $bindable(null),
		children,
		...rest
	}: Props = $props();
</script>

{#if href}
	<a bind:this={ref} {href} class={cn(buttonClass(variant, size), className as string)} {...rest}>
		{@render children?.()}
	</a>
{:else}
	<button
		bind:this={ref}
		{type}
		class={cn(buttonClass(variant, size), className as string)}
		{...rest}
	>
		{@render children?.()}
	</button>
{/if}
