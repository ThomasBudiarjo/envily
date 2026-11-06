<script lang="ts">
	import { page } from '$app/state';
	import Check from '@lucide/svelte/icons/check';
	import ChevronsUpDown from '@lucide/svelte/icons/chevrons-up-down';
	import Columns3 from '@lucide/svelte/icons/columns-3';
	import FolderKanban from '@lucide/svelte/icons/folder-kanban';
	import History from '@lucide/svelte/icons/history';
	import KeyRound from '@lucide/svelte/icons/key-round';
	import Layers from '@lucide/svelte/icons/layers';
	import ScrollText from '@lucide/svelte/icons/scroll-text';
	import Settings from '@lucide/svelte/icons/settings';
	import Users from '@lucide/svelte/icons/users';
	import { DropdownMenu } from 'bits-ui';
	import type { Project } from '#lib/api/types.js';
	import { t } from '#lib/i18n/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { cn } from '#lib/utils.js';

	// `brand` is off in the mobile drawer, whose header already shows the name.
	let { projects, brand = true }: { projects: Project[]; brand?: boolean } = $props();

	const slug = $derived(page.params.project);
	const current = $derived(projects.find((p) => p.slug === slug));
	const path = $derived(page.url.pathname);
	const canAudit = $derived(current?.role === 'owner' || current?.role === 'editor');

	const link = (active: boolean) =>
		cn(
			'flex h-8 items-center gap-2 rounded-md px-2 text-[13px] transition-colors [&_svg]:size-4 [&_svg]:shrink-0',
			active
				? 'bg-background font-medium text-foreground shadow-xs ring-1 ring-border'
				: 'text-muted-foreground hover:bg-muted hover:text-foreground'
		);
</script>

<nav class="flex h-full flex-col gap-5 overflow-y-auto p-3" aria-label={t.nav.projects}>
	{#if brand}
		<a href="/" class="flex items-center gap-2 px-2 pt-1">
			<span class="grid size-7 place-items-center rounded-lg bg-accent text-accent-foreground">
				<KeyRound class="size-3.5" />
			</span>
			<span class="font-semibold tracking-tight">{t.app.name}</span>
		</a>
	{/if}

	<DropdownMenu.Root>
		<DropdownMenu.Trigger
			class="flex h-9 w-full items-center gap-2 rounded-md border bg-background px-2 text-left text-[13px] outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
			data-testid="project-switcher"
		>
			<FolderKanban class="size-4 shrink-0 text-muted-foreground" />
			<span class="min-w-0 flex-1 truncate">{current?.name ?? t.nav.projects}</span>
			<ChevronsUpDown class="size-3.5 text-muted-foreground" />
		</DropdownMenu.Trigger>
		<DropdownMenu.Portal>
			<DropdownMenu.Content
				align="start"
				sideOffset={4}
				class="z-50 max-h-80 w-(--bits-dropdown-menu-anchor-width) min-w-52 overflow-y-auto rounded-lg border bg-popover p-1 shadow-md outline-none"
			>
				{#each projects as p (p.id)}
					<DropdownMenu.Item
						class="flex h-8 cursor-pointer items-center gap-2 rounded-md px-2 text-[13px] outline-none data-highlighted:bg-muted"
					>
						{#snippet child({ props })}
							<a href="/p/{p.slug}" {...props}>
								<span class="min-w-0 flex-1 truncate">{p.name}</span>
								{#if p.slug === slug}<Check class="size-4" />{/if}
							</a>
						{/snippet}
					</DropdownMenu.Item>
				{/each}
				<DropdownMenu.Separator class="-mx-1 my-1 h-px bg-border" />
				<DropdownMenu.Item
					class="flex h-8 cursor-pointer items-center gap-2 rounded-md px-2 text-[13px] text-muted-foreground outline-none data-highlighted:bg-muted"
				>
					{#snippet child({ props })}
						<a href="/" {...props}>{t.nav.projects}</a>
					{/snippet}
				</DropdownMenu.Item>
			</DropdownMenu.Content>
		</DropdownMenu.Portal>
	</DropdownMenu.Root>

	{#if current}
		<div class="grid gap-0.5">
			<p class="px-2 pb-1 text-[11px] font-medium tracking-wide text-muted-foreground uppercase">
				{t.nav.environments}
			</p>
			{#each current.environments as env (env.id)}
				{@const href = `/p/${current.slug}/${env.slug}`}
				<a {href} class={link(path === href)} aria-current={path === href ? 'page' : undefined}>
					<Layers />
					<span class="truncate">{env.name}</span>
				</a>
			{/each}
		</div>
		<div class="grid gap-0.5">
			<a href="/p/{current.slug}/compare" class={link(path.endsWith('/compare'))}>
				<Columns3 />{t.nav.compare}
			</a>
			{#if canAudit}
				<a href="/p/{current.slug}/audit" class={link(path.endsWith('/audit'))}>
					<History />{t.nav.audit}
				</a>
			{/if}
			<a href="/p/{current.slug}/settings" class={link(path.endsWith('/settings'))}>
				<Settings />{t.nav.settings}
			</a>
		</div>
	{/if}

	{#if session.isAdmin}
		<div class="mt-auto grid gap-0.5">
			<p class="px-2 pb-1 text-[11px] font-medium tracking-wide text-muted-foreground uppercase">
				{t.nav.admin}
			</p>
			<a href="/admin/users" class={link(path === '/admin/users')}><Users />{t.nav.users}</a>
			<a href="/admin/audit" class={link(path === '/admin/audit')}>
				<ScrollText />{t.nav.instanceAudit}
			</a>
		</div>
	{/if}
</nav>
