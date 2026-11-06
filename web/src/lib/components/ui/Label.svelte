<script lang="ts">
	import { Label } from 'bits-ui';
	import { cn } from '#lib/utils.js';

	let { class: className, ...rest }: Label.RootProps = $props();
</script>

<Label.Root class={cn('text-[13px] font-medium', className as string)} {...rest} />
