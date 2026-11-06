<script lang="ts">
	import KeyRound from '@lucide/svelte/icons/key-round';
	import Plus from '@lucide/svelte/icons/plus';
	import { toast } from 'svelte-sonner';
	import type { Project, ServiceToken, TokenPermission } from '#lib/api/types.js';
	import { createServiceToken, listServiceTokens, revokeServiceToken } from '#lib/api/tokens.js';
	import Badge from '#lib/components/ui/Badge.svelte';
	import Button from '#lib/components/ui/Button.svelte';
	import Checkbox from '#lib/components/ui/Checkbox.svelte';
	import ConfirmDialog from '#lib/components/ui/ConfirmDialog.svelte';
	import Dialog from '#lib/components/ui/Dialog.svelte';
	import EmptyState from '#lib/components/ui/EmptyState.svelte';
	import Field from '#lib/components/ui/Field.svelte';
	import Input from '#lib/components/ui/Input.svelte';
	import Label from '#lib/components/ui/Label.svelte';
	import OneTimeSecret from '#lib/components/ui/OneTimeSecret.svelte';
	import Select from '#lib/components/ui/Select.svelte';
	import { errorMessage } from '#lib/forms.js';
	import { t } from '#lib/i18n/index.js';
	import { absoluteTime, expiryFromDays, relativeTime } from '#lib/utils.js';

	let { project }: { project: Project } = $props();

	const expiries = [
		{ value: '0', label: t.settings.expiry.never },
		{ value: '7', label: t.settings.expiry.d7 },
		{ value: '30', label: t.settings.expiry.d30 },
		{ value: '90', label: t.settings.expiry.d90 },
		{ value: '365', label: t.settings.expiry.d365 }
	];

	let tokens = $state<ServiceToken[]>([]);
	let open = $state(false);
	let name = $state('');
	let envs = $state<Record<string, boolean>>({});
	let permission = $state<TokenPermission>('read');
	let expiry = $state('90');
	let created = $state('');
	let busy = $state(false);
	let toRevoke = $state<ServiceToken | null>(null);
	let revokeOpen = $state(false);

	async function load() {
		try {
			tokens = await listServiceTokens(project.slug);
		} catch (err) {
			toast.error(errorMessage(err));
		}
	}

	$effect(() => {
		void project.slug;
		void load();
	});

	function openCreate() {
		name = '';
		envs = {};
		permission = 'read';
		expiry = '90';
		created = '';
		open = true;
	}

	const chosen = $derived(project.environments.filter((e) => envs[e.slug]).map((e) => e.slug));

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!chosen.length) {
			toast.error(t.settings.pickEnvs);
			return;
		}
		busy = true;
		try {
			const r = await createServiceToken(project.slug, {
				name: name.trim(),
				environments: chosen,
				permission,
				expiresAt: expiryFromDays(Number(expiry))
			});
			created = r.token;
			await load();
		} catch (err) {
			toast.error(errorMessage(err));
		} finally {
			busy = false;
		}
	}

	async function revoke() {
		if (!toRevoke) return;
		try {
			await revokeServiceToken(project.slug, toRevoke.id);
			await load();
		} catch (err) {
			toast.error(errorMessage(err));
			throw err;
		}
	}

	function status(tk: ServiceToken) {
		if (tk.revokedAt) return { label: t.common.revoked, tone: 'neutral' as const };
		if (tk.expiresAt && Date.parse(tk.expiresAt) < Date.now())
			return { label: t.common.expired, tone: 'warning' as const };
		return { label: t.common.active, tone: 'success' as const };
	}
</script>

<div class="grid max-w-3xl gap-4">
	<div><Button onclick={openCreate}><Plus />{t.settings.createToken}</Button></div>
	{#if tokens.length === 0}
		<EmptyState icon={KeyRound} title={t.settings.noTokens} />
	{:else}
		<ul class="divide-y rounded-lg border bg-card" data-testid="token-list">
			{#each tokens as tk (tk.id)}
				{@const st = status(tk)}
				<li class="flex flex-wrap items-center gap-3 px-3 py-2.5 sm:flex-nowrap">
					<div class="min-w-0 flex-1">
						<p class="flex items-center gap-2 font-medium">
							<span class="truncate">{tk.name}</span>
							<Badge tone={st.tone}>{st.label}</Badge>
						</p>
						<p class="truncate text-xs text-muted-foreground">
							<span class="font-mono">{tk.environments.join(', ')}</span>
							· {tk.permission === 'read' ? t.settings.permRead : t.settings.permReadWrite}
							· {t.common.expires}
							{tk.expiresAt ? absoluteTime(tk.expiresAt) : t.common.never}
							· {t.common.lastUsed}
							{tk.lastUsedAt ? relativeTime(tk.lastUsedAt) : t.common.never}
						</p>
					</div>
					{#if !tk.revokedAt}
						<Button
							variant="outline"
							size="sm"
							onclick={() => {
								toRevoke = tk;
								revokeOpen = true;
							}}
						>
							{t.common.revoke}
						</Button>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</div>

<Dialog bind:open title={t.settings.createToken}>
	{#if created}
		<OneTimeSecret value={created} note={t.settings.tokenCreated} />
	{:else}
		<form id="create-token" class="grid gap-4" onsubmit={submit}>
			<Field id="token-name" label={t.settings.tokenName}>
				<Input id="token-name" bind:value={name} required maxlength={64} autocomplete="off" />
			</Field>
			<fieldset class="grid gap-2">
				<legend class="mb-1 text-[13px] font-medium">{t.settings.tokenEnvs}</legend>
				{#each project.environments as env (env.id)}
					<div class="flex items-center gap-2">
						<Checkbox id="token-env-{env.slug}" bind:checked={envs[env.slug]} />
						<Label for="token-env-{env.slug}" class="font-normal">{env.name}</Label>
					</div>
				{/each}
			</fieldset>
			<Field id="token-permission" label={t.settings.tokenPermission}>
				<Select
					id="token-permission"
					bind:value={permission}
					items={[
						{ value: 'read', label: t.settings.permRead },
						{ value: 'read_write', label: t.settings.permReadWrite }
					]}
				/>
			</Field>
			<Field id="token-expiry" label={t.settings.tokenExpires}>
				<Select id="token-expiry" bind:value={expiry} items={expiries} />
			</Field>
		</form>
	{/if}
	{#snippet footer()}
		{#if created}
			<Button onclick={() => (open = false)}>{t.common.close}</Button>
		{:else}
			<Button variant="outline" onclick={() => (open = false)}>{t.common.cancel}</Button>
			<Button type="submit" form="create-token" disabled={busy || !name.trim()}>
				{busy ? t.common.creating : t.common.create}
			</Button>
		{/if}
	{/snippet}
</Dialog>

<ConfirmDialog
	bind:open={revokeOpen}
	title={t.settings.revokeTokenTitle(toRevoke?.name ?? '')}
	description={t.settings.revokeTokenIntro}
	confirmLabel={t.common.revoke}
	onconfirm={revoke}
/>
