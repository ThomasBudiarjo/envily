<script lang="ts">
	import { AlertDialog } from 'bits-ui';
	import { buttonClass } from './Button.svelte';
	import Input from './Input.svelte';
	import { t } from '#lib/i18n/index.js';

	// Confirmation for destructive actions. With `confirmText`, the user must
	// type it (e.g. the project slug) before the action is enabled.
	let {
		open = $bindable(false),
		title,
		description,
		confirmLabel = t.common.delete,
		confirmText,
		destructive = true,
		onconfirm
	}: {
		open?: boolean;
		title: string;
		description?: string;
		confirmLabel?: string;
		confirmText?: string;
		destructive?: boolean;
		onconfirm: () => Promise<unknown> | unknown;
	} = $props();

	let typed = $state('');
	let busy = $state(false);
	const ready = $derived(!confirmText || typed === confirmText);

	$effect(() => {
		if (open) typed = '';
	});

	async function confirm(e: Event) {
		e.preventDefault();
		if (!ready || busy) return;
		busy = true;
		try {
			await onconfirm();
			open = false;
		} finally {
			busy = false;
		}
	}
</script>

<AlertDialog.Root bind:open>
	<AlertDialog.Portal>
		<AlertDialog.Overlay class="fixed inset-0 z-50 bg-black/40" />
		<AlertDialog.Content
			class="fixed top-1/2 left-1/2 z-50 grid w-[calc(100vw-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 gap-4 rounded-lg border bg-popover p-5 shadow-lg outline-none"
		>
			<form class="grid gap-4" onsubmit={confirm}>
				<div class="grid gap-1">
					<AlertDialog.Title class="text-base font-semibold">{title}</AlertDialog.Title>
					{#if description}
						<AlertDialog.Description class="text-sm text-muted-foreground">
							{description}
						</AlertDialog.Description>
					{/if}
				</div>
				{#if confirmText}
					<div class="grid gap-1.5">
						<p class="text-[13px]">{t.common.typeToConfirm(confirmText)}</p>
						<Input
							bind:value={typed}
							autocomplete="off"
							aria-label={t.common.typeToConfirm(confirmText)}
							class="font-mono"
						/>
					</div>
				{/if}
				<div class="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
					<AlertDialog.Cancel type="button" class={buttonClass('outline')}>
						{t.common.cancel}
					</AlertDialog.Cancel>
					<button
						type="submit"
						class={buttonClass(destructive ? 'destructive' : 'primary')}
						disabled={!ready || busy}
					>
						{confirmLabel}
					</button>
				</div>
			</form>
		</AlertDialog.Content>
	</AlertDialog.Portal>
</AlertDialog.Root>
