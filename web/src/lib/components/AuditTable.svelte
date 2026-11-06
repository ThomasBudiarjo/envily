<script lang="ts">
	import ScrollText from '@lucide/svelte/icons/scroll-text';
	import { untrack } from 'svelte';
	import { toast } from 'svelte-sonner';
	import type { AuditEntry, AuditFilter, Page } from '#lib/api/types.js';
	import Button from '#lib/components/ui/Button.svelte';
	import EmptyState from '#lib/components/ui/EmptyState.svelte';
	import Input from '#lib/components/ui/Input.svelte';
	import Label from '#lib/components/ui/Label.svelte';
	import Select from '#lib/components/ui/Select.svelte';
	import { errorMessage } from '#lib/forms.js';
	import { t } from '#lib/i18n/index.js';
	import { absoluteTime, relativeTime } from '#lib/utils.js';

	// Audit log with filters and infinite scroll (an IntersectionObserver on
	// a sentinel row loads the next cursor page).
	let { fetchPage }: { fetchPage: (f: AuditFilter) => Promise<Page<AuditEntry>> } = $props();

	const actions = [
		'auth.login',
		'auth.login_failed',
		'auth.logout',
		'auth.password_changed',
		'user.created',
		'user.updated',
		'user.disabled',
		'project.created',
		'project.updated',
		'project.deleted',
		'member.added',
		'member.updated',
		'member.removed',
		'environment.created',
		'environment.updated',
		'environment.deleted',
		'secret.created',
		'secret.updated',
		'secret.renamed',
		'secret.deleted',
		'secret.read',
		'secrets.read_all',
		'secrets.imported',
		'secrets.export',
		'secret.rolled_back',
		'service_token.created',
		'service_token.revoked',
		'personal_token.created',
		'personal_token.revoked'
	];

	let action = $state('');
	let actor = $state('');
	let from = $state('');
	let to = $state('');
	let items = $state<AuditEntry[]>([]);
	let cursor = $state<string | null>(null);
	let done = $state(false);
	let loading = $state(false);
	let generation = 0;
	let sentinel = $state<HTMLElement | null>(null);

	const filter = $derived<AuditFilter>({
		action: action || undefined,
		actor: actor.trim() || undefined,
		from: from ? new Date(from + 'T00:00:00').toISOString() : undefined,
		to: to ? new Date(to + 'T23:59:59.999').toISOString() : undefined,
		limit: 50
	});

	async function more() {
		if (loading || done) return;
		loading = true;
		const gen = generation;
		try {
			const page = await fetchPage({ ...filter, cursor: cursor ?? undefined });
			if (gen !== generation) return;
			items.push(...page.items);
			cursor = page.nextCursor;
			done = !page.nextCursor;
		} catch (e) {
			toast.error(errorMessage(e));
			done = true;
		} finally {
			loading = false;
		}
	}

	// Restart from the first page whenever the filter changes. Only `filter` is
	// a dependency; more() reads and writes the paging state, so it runs untracked.
	$effect(() => {
		void filter;
		untrack(() => {
			generation++;
			items = [];
			cursor = null;
			done = false;
			loading = false;
			void more();
		});
	});

	$effect(() => {
		if (!sentinel) return;
		const io = new IntersectionObserver((e) => {
			if (e[0].isIntersecting) void more();
		});
		io.observe(sentinel);
		return () => io.disconnect();
	});

	function describe(e: AuditEntry): string {
		// Metadata holds names, keys, and version numbers only (never values).
		return Object.entries(e.metadata ?? {})
			.filter(([, v]) => v !== null && v !== '')
			.map(([k, v]) => `${k}=${Array.isArray(v) ? v.join(',') : String(v)}`)
			.join(' ');
	}
</script>

<div class="mb-4 grid grid-cols-2 gap-3 lg:grid-cols-4">
	<div class="grid gap-1.5">
		<Label for="audit-action">{t.audit.filterAction}</Label>
		<Select
			id="audit-action"
			bind:value={action}
			items={[
				{ value: '', label: t.audit.allActions },
				...actions.map((a) => ({ value: a, label: a }))
			]}
		/>
	</div>
	<div class="grid gap-1.5">
		<Label for="audit-actor">{t.audit.filterActor}</Label>
		<Input id="audit-actor" bind:value={actor} class="font-mono text-xs" autocomplete="off" />
	</div>
	<div class="grid gap-1.5">
		<Label for="audit-from">{t.audit.from}</Label>
		<Input id="audit-from" type="date" bind:value={from} />
	</div>
	<div class="grid gap-1.5">
		<Label for="audit-to">{t.audit.to}</Label>
		<Input id="audit-to" type="date" bind:value={to} />
	</div>
</div>

{#if done && items.length === 0}
	<EmptyState icon={ScrollText} title={t.audit.empty} />
{:else}
	<div class="overflow-x-auto rounded-lg border bg-card">
		<ul class="divide-y md:hidden">
			{#each items as e (e.id)}
				<li class="grid gap-0.5 px-3 py-2.5 text-[13px]">
					<div class="flex items-center justify-between gap-2">
						<span class="font-mono text-xs font-medium">{e.action}</span>
						<span class="shrink-0 text-xs text-muted-foreground" title={absoluteTime(e.at)}
							>{relativeTime(e.at)}</span
						>
					</div>
					<p class="truncate">{e.actorLabel || e.actorType}</p>
					{#if describe(e)}
						<p class="font-mono text-xs break-all text-muted-foreground">{describe(e)}</p>
					{/if}
				</li>
			{/each}
		</ul>
		<table class="hidden w-full text-[13px] md:table" data-testid="audit-table">
			<thead class="bg-muted/40 text-left text-xs text-muted-foreground">
				<tr>
					<th class="px-3 py-2 font-medium">{t.audit.when}</th>
					<th class="px-3 py-2 font-medium">{t.audit.actor}</th>
					<th class="px-3 py-2 font-medium">{t.audit.action}</th>
					<th class="px-3 py-2 font-medium">{t.audit.details}</th>
				</tr>
			</thead>
			<tbody>
				{#each items as e (e.id)}
					<tr class="border-t align-top">
						<td class="px-3 py-2 whitespace-nowrap" title={absoluteTime(e.at)}
							>{relativeTime(e.at)}</td
						>
						<td class="px-3 py-2">
							<p class="max-w-56 truncate">{e.actorLabel || e.actorType}</p>
							<p class="text-xs text-muted-foreground">{e.actorType}{e.ip ? ` · ${e.ip}` : ''}</p>
						</td>
						<td class="px-3 py-2 font-mono text-xs whitespace-nowrap">{e.action}</td>
						<td class="px-3 py-2 font-mono text-xs break-all text-muted-foreground"
							>{describe(e)}</td
						>
					</tr>
				{/each}
			</tbody>
		</table>
		<div bind:this={sentinel} class="px-3 py-3 text-center text-xs text-muted-foreground">
			{#if loading}{t.common.loading}{:else if done}{t.audit.end}{:else}
				<Button variant="ghost" size="sm" onclick={more}>{t.common.loadMore}</Button>
			{/if}
		</div>
	</div>
{/if}
