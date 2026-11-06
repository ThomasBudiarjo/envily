<script lang="ts">
	import { Dialog } from 'bits-ui';
	import X from '@lucide/svelte/icons/x';
	import type { Snippet } from 'svelte';
	import { t } from '#lib/i18n/index.js';
	import { cn } from '#lib/utils.js';

	// A dialog that slides in from the side: version history, mobile nav.
	let {
		open = $bindable(false),
		title,
		side = 'right',
		class: className,
		children
	}: {
		open?: boolean;
		title: string;
		side?: 'left' | 'right';
		class?: string;
		children: Snippet;
	} = $props();
</script>

<Dialog.Root bind:open>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-50 bg-black/40" />
		<Dialog.Content
			class={cn(
				'fixed inset-y-0 z-50 flex w-[min(28rem,100vw)] flex-col border-border bg-popover shadow-lg outline-none',
				side === 'right' ? 'right-0 border-l' : 'left-0 border-r',
				className
			)}
		>
			<div class="flex h-14 shrink-0 items-center justify-between border-b px-4">
				<Dialog.Title class="truncate text-sm font-semibold">{title}</Dialog.Title>
				<Dialog.Close
					class="rounded-md p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
					aria-label={t.common.close}
				>
					<X class="size-4" />
				</Dialog.Close>
			</div>
			<div class="min-h-0 flex-1 overflow-y-auto">{@render children()}</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
