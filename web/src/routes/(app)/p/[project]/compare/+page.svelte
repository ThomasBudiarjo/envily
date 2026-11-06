<script lang="ts">
	import Check from '@lucide/svelte/icons/check';
	import Columns3 from '@lucide/svelte/icons/columns-3';
	import Minus from '@lucide/svelte/icons/minus';
	import EmptyState from '#lib/components/ui/EmptyState.svelte';
	import Input from '#lib/components/ui/Input.svelte';
	import Switch from '#lib/components/ui/Switch.svelte';
	import { t } from '#lib/i18n/index.js';
	import { absoluteTime, cn } from '#lib/utils.js';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	let onlyGaps = $state(false);
	let query = $state('');

	const envs = $derived(data.comparison.environments);
	const rows = $derived.by(() => {
		const q = query.trim().toLowerCase();
		return Object.entries(data.comparison.keys)
			.map(([key, cells]) => {
				const present = envs.filter((e) => cells[e.slug]?.exists).length;
				return { key, cells, gap: present > 0 && present < envs.length };
			})
			.filter((r) => (!onlyGaps || r.gap) && (!q || r.key.toLowerCase().includes(q)))
			.sort((a, b) => a.key.localeCompare(b.key));
	});
	const gapCount = $derived(
		Object.values(data.comparison.keys).filter((cells) => {
			const n = envs.filter((e) => cells[e.slug]?.exists).length;
			return n > 0 && n < envs.length;
		}).length
	);
</script>

<svelte:head><title>{t.compare.title} · {data.project.name} · {t.app.name}</title></svelte:head>

<div class="mb-5">
	<h1 class="text-xl font-semibold tracking-tight">{t.compare.title}</h1>
	<p class="mt-1 text-[13px] text-muted-foreground">{t.compare.intro}</p>
</div>

{#if Object.keys(data.comparison.keys).length === 0}
	<EmptyState icon={Columns3} title={t.compare.empty} />
{:else}
	<div class="mb-4 flex flex-wrap items-center gap-4">
		<Input
			bind:value={query}
			placeholder={t.secrets.searchPlaceholder}
			aria-label={t.common.search}
			class="max-w-xs"
			type="search"
		/>
		<label class="flex items-center gap-2 text-[13px]">
			<Switch bind:checked={onlyGaps} label={t.compare.onlyMissing} />
			{t.compare.onlyMissing} ({gapCount})
		</label>
	</div>
	<div class="overflow-x-auto rounded-lg border bg-card">
		<table class="w-full text-[13px]" data-testid="compare-table">
			<thead class="bg-muted/40 text-xs text-muted-foreground">
				<tr>
					<th class="sticky left-0 bg-muted/40 px-3 py-2 text-left font-medium">{t.secrets.key}</th>
					{#each envs as e (e.id)}
						<th class="px-3 py-2 text-center font-medium whitespace-nowrap">
							<a href="/p/{data.project.slug}/{e.slug}" class="hover:text-foreground">{e.name}</a>
						</th>
					{/each}
				</tr>
			</thead>
			<tbody>
				{#each rows as r (r.key)}
					<tr class={cn('border-t', r.gap && 'bg-warning/6')} data-gap={r.gap}>
						<td
							class={cn(
								'sticky left-0 px-3 py-2 font-mono font-medium whitespace-nowrap',
								r.gap
									? 'bg-[color-mix(in_oklab,var(--color-warning)_6%,var(--color-card))]'
									: 'bg-card'
							)}
						>
							{r.key}
						</td>
						{#each envs as e (e.id)}
							{@const c = r.cells[e.slug]}
							<td class="px-3 py-2 text-center">
								{#if c?.exists}
									<span
										class="inline-flex items-center gap-1 text-success"
										title={c.updatedAt ? absoluteTime(c.updatedAt) : undefined}
									>
										<Check class="size-4" /><span class="sr-only">{t.compare.present}</span>
										<span class="text-xs text-muted-foreground">v{c.version}</span>
									</span>
								{:else}
									<span
										class={cn(
											'inline-flex items-center gap-1',
											r.gap ? 'font-medium text-destructive' : 'text-muted-foreground'
										)}
									>
										<Minus class="size-4" /><span class="text-xs">{t.compare.missing}</span>
									</span>
								{/if}
							</td>
						{/each}
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
