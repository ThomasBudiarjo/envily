<script lang="ts">
	import Copy from '@lucide/svelte/icons/copy';
	import { toast } from 'svelte-sonner';
	import IconButton from './IconButton.svelte';
	import { t } from '#lib/i18n/index.js';

	// Shows a token or temporary password once, with a copy button.
	let { value, note }: { value: string; note: string } = $props();

	async function copy() {
		await navigator.clipboard.writeText(value);
		toast.success(t.common.copied);
	}
</script>

<div class="grid gap-2 rounded-lg border border-warning/40 bg-warning/8 p-3">
	<p class="text-[13px]">{note}</p>
	<div class="flex items-center gap-2">
		<code
			class="min-w-0 flex-1 rounded-md border bg-background px-2 py-1.5 font-mono text-xs break-all select-all"
			data-testid="one-time-secret">{value}</code
		>
		<IconButton label={t.common.copy} onclick={copy}><Copy /></IconButton>
	</div>
</div>
