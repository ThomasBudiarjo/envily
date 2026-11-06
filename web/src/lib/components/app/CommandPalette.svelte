<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { Command, Dialog } from 'bits-ui';
	import Columns3 from '@lucide/svelte/icons/columns-3';
	import FolderKanban from '@lucide/svelte/icons/folder-kanban';
	import Layers from '@lucide/svelte/icons/layers';
	import Search from '@lucide/svelte/icons/search';
	import Settings from '@lucide/svelte/icons/settings';
	import UserRound from '@lucide/svelte/icons/user-round';
	import Users from '@lucide/svelte/icons/users';
	import type { Component } from 'svelte';
	import type { Project } from '#lib/api/types.js';
	import { t } from '#lib/i18n/index.js';
	import { session } from '#lib/stores/session.svelte.js';

	let { open = $bindable(false), projects }: { open?: boolean; projects: Project[] } = $props();

	interface Entry {
		value: string;
		label: string;
		hint?: string;
		href: string;
		icon: Component;
	}

	const current = $derived(projects.find((p) => p.slug === page.params.project));

	const groups = $derived.by(() => {
		const out: { heading: string; items: Entry[] }[] = [];
		if (current) {
			out.push({
				heading: `${t.command.environments} · ${current.name}`,
				items: [
					...current.environments.map((e) => ({
						value: `env ${current.slug} ${e.slug}`,
						label: e.name,
						hint: e.slug,
						href: `/p/${current.slug}/${e.slug}`,
						icon: Layers
					})),
					{
						value: `compare ${current.slug}`,
						label: t.nav.compare,
						href: `/p/${current.slug}/compare`,
						icon: Columns3
					},
					{
						value: `settings ${current.slug}`,
						label: t.nav.settings,
						href: `/p/${current.slug}/settings`,
						icon: Settings
					}
				]
			});
		}
		out.push({
			heading: t.command.projects,
			items: projects.map((p) => ({
				value: `project ${p.slug} ${p.name}`,
				label: p.name,
				hint: p.slug,
				href: `/p/${p.slug}`,
				icon: FolderKanban
			}))
		});
		const pages: Entry[] = [
			{ value: 'page projects', label: t.nav.projects, href: '/', icon: FolderKanban },
			{ value: 'page account', label: t.nav.account, href: '/account', icon: UserRound }
		];
		if (session.isAdmin)
			pages.push({ value: 'page users', label: t.nav.users, href: '/admin/users', icon: Users });
		out.push({ heading: t.command.pages, items: pages });
		return out;
	});

	function select(href: string) {
		open = false;
		void goto(href);
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-50 bg-black/40" />
		<Dialog.Content
			class="fixed top-[12dvh] left-1/2 z-50 w-[calc(100vw-2rem)] max-w-lg -translate-x-1/2 overflow-hidden rounded-xl border bg-popover shadow-xl outline-none"
		>
			<Dialog.Title class="sr-only">{t.nav.commandPalette}</Dialog.Title>
			<Command.Root class="flex flex-col">
				<div class="flex items-center gap-2 border-b px-3">
					<Search class="size-4 text-muted-foreground" />
					<Command.Input
						placeholder={t.command.placeholder}
						class="h-11 flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
					/>
				</div>
				<Command.List class="max-h-80 overflow-y-auto p-1.5">
					<Command.Viewport>
						<Command.Empty class="py-8 text-center text-[13px] text-muted-foreground">
							{t.command.empty}
						</Command.Empty>
						{#each groups as g (g.heading)}
							<Command.Group>
								<Command.GroupHeading
									class="px-2 pt-2 pb-1 text-[11px] font-medium text-muted-foreground"
								>
									{g.heading}
								</Command.GroupHeading>
								<Command.GroupItems>
									{#each g.items as item (item.value)}
										<Command.Item
											value={item.value}
											keywords={[item.label]}
											onSelect={() => select(item.href)}
											class="flex h-9 cursor-pointer items-center gap-2 rounded-md px-2 text-[13px] outline-none select-none data-selected:bg-muted [&_svg]:size-4 [&_svg]:text-muted-foreground"
										>
											<item.icon />
											<span class="truncate">{item.label}</span>
											{#if item.hint}
												<span class="ml-auto font-mono text-xs text-muted-foreground"
													>{item.hint}</span
												>
											{/if}
										</Command.Item>
									{/each}
								</Command.GroupItems>
							</Command.Group>
						{/each}
					</Command.Viewport>
				</Command.List>
			</Command.Root>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
