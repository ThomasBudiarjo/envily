<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import ChevronRight from '@lucide/svelte/icons/chevron-right';
	import LogOut from '@lucide/svelte/icons/log-out';
	import Menu from '@lucide/svelte/icons/menu';
	import Monitor from '@lucide/svelte/icons/monitor';
	import Moon from '@lucide/svelte/icons/moon';
	import Search from '@lucide/svelte/icons/search';
	import Sun from '@lucide/svelte/icons/sun';
	import UserRound from '@lucide/svelte/icons/user-round';
	import { logout } from '#lib/api/auth.js';
	import type { Project } from '#lib/api/types.js';
	import DropdownMenu from '#lib/components/ui/DropdownMenu.svelte';
	import IconButton from '#lib/components/ui/IconButton.svelte';
	import { t } from '#lib/i18n/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { theme } from '#lib/stores/theme.svelte.js';

	let { onmenu, onpalette }: { onmenu: () => void; onpalette: () => void } = $props();

	const shortcut = /Mac|iPhone|iPad/.test(navigator.userAgent) ? '⌘K' : 'Ctrl K';

	const project = $derived(page.data.project as Project | undefined);
	const crumbs = $derived.by(() => {
		const out: { label: string; href?: string }[] = [];
		const path = page.url.pathname;
		if (project) {
			out.push({ label: project.name, href: `/p/${project.slug}` });
			const envSlug = (page.params as Record<string, string | undefined>).env;
			const env = project.environments.find((e) => e.slug === envSlug);
			if (env) out.push({ label: env.name });
			else if (path.endsWith('/compare')) out.push({ label: t.nav.compare });
			else if (path.endsWith('/settings')) out.push({ label: t.nav.settings });
			else if (path.endsWith('/audit')) out.push({ label: t.nav.audit });
		} else if (path === '/account') out.push({ label: t.nav.account });
		else if (path === '/admin/users') out.push({ label: t.nav.users });
		else if (path === '/admin/audit') out.push({ label: t.nav.instanceAudit });
		else out.push({ label: t.nav.projects });
		return out;
	});

	const ThemeIcon = $derived(
		theme.choice === 'light' ? Sun : theme.choice === 'dark' ? Moon : Monitor
	);
	const themeLabel = $derived(
		`${t.nav.toggleTheme} (${theme.choice === 'light' ? t.nav.themeLight : theme.choice === 'dark' ? t.nav.themeDark : t.nav.themeSystem})`
	);

	async function signOut() {
		await logout().catch(() => {});
		session.clear();
		session.loaded = false;
		await goto('/login', { replace: true });
	}
</script>

<header
	class="sticky top-0 z-30 flex h-14 items-center gap-2 border-b bg-background/85 px-3 backdrop-blur sm:px-4"
>
	<IconButton label={t.nav.menu} class="lg:hidden" onclick={onmenu}><Menu /></IconButton>
	<nav aria-label="Breadcrumb" class="min-w-0 flex-1">
		<ol class="flex min-w-0 items-center gap-1 text-[13px]">
			{#each crumbs as c, i (i)}
				{#if i > 0}<ChevronRight class="size-3.5 shrink-0 text-muted-foreground" />{/if}
				<li
					class="min-w-0 truncate {i === crumbs.length - 1
						? 'font-medium'
						: 'text-muted-foreground'}"
				>
					{#if c.href && i < crumbs.length - 1}
						<a href={c.href} class="hover:text-foreground">{c.label}</a>
					{:else}
						{c.label}
					{/if}
				</li>
			{/each}
		</ol>
	</nav>
	<button
		type="button"
		onclick={onpalette}
		class="hidden h-8 items-center gap-2 rounded-md border bg-muted/40 px-2.5 text-[13px] text-muted-foreground hover:bg-muted sm:flex"
	>
		<Search class="size-3.5" />
		<span>{t.nav.commandPalette}</span>
		<kbd class="ml-4 rounded border bg-background px-1 font-mono text-[10px]">{shortcut}</kbd>
	</button>
	<IconButton label={t.nav.commandPalette} class="sm:hidden" onclick={onpalette}
		><Search /></IconButton
	>
	<IconButton label={themeLabel} onclick={() => theme.toggle()} data-testid="theme-toggle">
		<ThemeIcon />
	</IconButton>
	<DropdownMenu
		items={[
			{ label: t.nav.account, icon: UserRound, onselect: () => goto('/account') },
			{ label: t.nav.logout, icon: LogOut, onselect: signOut, separatorBefore: true }
		]}
	>
		{#snippet header()}
			<p class="truncate text-[13px] font-medium">{session.user?.name}</p>
			<p class="truncate text-xs text-muted-foreground">{session.user?.email}</p>
		{/snippet}
		{#snippet trigger({ props })}
			<button
				{...props}
				type="button"
				aria-label={t.nav.userMenu}
				class="grid size-8 place-items-center rounded-full bg-accent/15 text-xs font-semibold text-accent outline-none focus-visible:ring-2 focus-visible:ring-ring"
			>
				{(session.user?.name || session.user?.email || '?').slice(0, 1).toUpperCase()}
			</button>
		{/snippet}
	</DropdownMenu>
</header>
