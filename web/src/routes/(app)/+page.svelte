<script lang="ts">
	import FolderKanban from '@lucide/svelte/icons/folder-kanban';
	import Plus from '@lucide/svelte/icons/plus';
	import CreateProjectDialog from '#lib/components/app/CreateProjectDialog.svelte';
	import Badge from '#lib/components/ui/Badge.svelte';
	import Button from '#lib/components/ui/Button.svelte';
	import EmptyState from '#lib/components/ui/EmptyState.svelte';
	import { t } from '#lib/i18n/index.js';
	import { absoluteTime, relativeTime } from '#lib/utils.js';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let creating = $state(false);
</script>

<svelte:head><title>{t.projects.title} · {t.app.name}</title></svelte:head>

<div class="mb-6 flex items-center justify-between gap-4">
	<h1 class="text-xl font-semibold tracking-tight">{t.projects.title}</h1>
	{#if data.projects.length}
		<Button onclick={() => (creating = true)}><Plus />{t.projects.new}</Button>
	{/if}
</div>

{#if data.projects.length === 0}
	<EmptyState icon={FolderKanban} title={t.projects.empty} description={t.projects.emptyIntro}>
		<Button onclick={() => (creating = true)}><Plus />{t.projects.new}</Button>
	</EmptyState>
{:else}
	<ul class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
		{#each data.projects as p (p.id)}
			<li>
				<a
					href="/p/{p.slug}"
					class="group grid h-full gap-3 rounded-lg border bg-card p-4 transition-colors hover:border-accent/50 hover:bg-muted/30"
				>
					<div class="flex items-start justify-between gap-2">
						<div class="min-w-0">
							<p class="truncate font-medium group-hover:text-accent">{p.name}</p>
							<p class="truncate font-mono text-xs text-muted-foreground">{p.slug}</p>
						</div>
						<Badge>{p.role ? t.projects.role[p.role] : t.projects.role.admin}</Badge>
					</div>
					<div class="flex items-center justify-between text-xs text-muted-foreground">
						<span>{t.projects.environments(p.environments.length)}</span>
						<span title={absoluteTime(p.updatedAt)}
							>{t.projects.updated(relativeTime(p.updatedAt))}</span
						>
					</div>
				</a>
			</li>
		{/each}
	</ul>
{/if}

<CreateProjectDialog bind:open={creating} />
