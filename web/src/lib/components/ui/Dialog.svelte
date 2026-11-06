<script lang="ts">
	import { Dialog } from 'bits-ui';
	import X from '@lucide/svelte/icons/x';
	import type { Snippet } from 'svelte';
	import { t } from '#lib/i18n/index.js';
	import { cn } from '#lib/utils.js';

	let {
		open = $bindable(false),
		title,
		description,
		class: className,
		children,
		footer
	}: {
		open?: boolean;
		title: string;
		description?: string;
		class?: string;
		children: Snippet;
		footer?: Snippet;
	} = $props();
</script>

<Dialog.Root bind:open>
	<Dialog.Portal>
		<Dialog.Overlay class="data-[state=open]:animate-in fixed inset-0 z-50 bg-black/40" />
		<Dialog.Content
			class={cn(
				'fixed top-1/2 left-1/2 z-50 grid max-h-[calc(100dvh-2rem)] w-[calc(100vw-2rem)] max-w-lg -translate-x-1/2 -translate-y-1/2 gap-4 overflow-y-auto rounded-lg border bg-popover p-5 shadow-lg outline-none',
				className
			)}
		>
			<div class="grid gap-1 pr-6">
				<Dialog.Title class="text-base font-semibold">{title}</Dialog.Title>
				{#if description}
					<Dialog.Description class="text-sm text-muted-foreground"
						>{description}</Dialog.Description
					>
				{/if}
			</div>
			{@render children()}
			{#if footer}
				<div class="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">{@render footer()}</div>
			{/if}
			<Dialog.Close
				class="absolute top-4 right-4 rounded-md p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
				aria-label={t.common.close}
			>
				<X class="size-4" />
			</Dialog.Close>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
