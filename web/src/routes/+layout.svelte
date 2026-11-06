<script lang="ts">
	import '../app.css';
	import { Tooltip } from 'bits-ui';
	import { Toaster } from 'svelte-sonner';
	import { theme } from '#lib/stores/theme.svelte.js';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();
</script>

<Tooltip.Provider>
	{@render children()}
</Tooltip.Provider>
<Toaster theme={theme.dark ? 'dark' : 'light'} position="bottom-right" richColors closeButton />
