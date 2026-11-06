<script lang="ts">
	import KeyRound from '@lucide/svelte/icons/key-round';
	import Plus from '@lucide/svelte/icons/plus';
	import { toast } from 'svelte-sonner';
	import { createMyToken, listMyTokens, revokeMyToken } from '#lib/api/auth.js';
	import type { PersonalToken } from '#lib/api/types.js';
	import PasswordForm from '#lib/components/PasswordForm.svelte';
	import Badge from '#lib/components/ui/Badge.svelte';
	import Button from '#lib/components/ui/Button.svelte';
	import ConfirmDialog from '#lib/components/ui/ConfirmDialog.svelte';
	import Dialog from '#lib/components/ui/Dialog.svelte';
	import EmptyState from '#lib/components/ui/EmptyState.svelte';
	import Field from '#lib/components/ui/Field.svelte';
	import Input from '#lib/components/ui/Input.svelte';
	import OneTimeSecret from '#lib/components/ui/OneTimeSecret.svelte';
	import Select from '#lib/components/ui/Select.svelte';
	import { errorMessage } from '#lib/forms.js';
	import { t } from '#lib/i18n/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { absoluteTime, expiryFromDays, relativeTime } from '#lib/utils.js';

	let tokens = $state<PersonalToken[]>([]);
	let open = $state(false);
	let name = $state('');
	let expiry = $state('90');
	let created = $state('');
	let busy = $state(false);
	let toRevoke = $state<PersonalToken | null>(null);
	let revokeOpen = $state(false);

	async function load() {
		try {
			tokens = await listMyTokens();
		} catch (err) {
			toast.error(errorMessage(err));
		}
	}
	$effect(() => void load());

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		try {
			const r = await createMyToken(name.trim(), expiryFromDays(Number(expiry)));
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
			await revokeMyToken(toRevoke.id);
			await load();
		} catch (err) {
			toast.error(errorMessage(err));
			throw err;
		}
	}

	const active = (tk: PersonalToken) =>
		!tk.revokedAt && !(tk.expiresAt && Date.parse(tk.expiresAt) < Date.now());
</script>

<svelte:head><title>{t.account.title} · {t.app.name}</title></svelte:head>

<h1 class="mb-6 text-xl font-semibold tracking-tight">{t.account.title}</h1>

<div class="grid gap-10">
	<section class="grid max-w-md gap-2">
		<h2 class="text-sm font-semibold">{t.account.profile}</h2>
		<dl class="grid grid-cols-[6rem_1fr] gap-y-1 text-[13px]">
			<dt class="text-muted-foreground">{t.common.name}</dt>
			<dd>{session.user?.name}</dd>
			<dt class="text-muted-foreground">{t.common.email}</dt>
			<dd>{session.user?.email}</dd>
		</dl>
	</section>

	<section class="grid max-w-md gap-4">
		<h2 class="text-sm font-semibold">{t.account.changePassword}</h2>
		<PasswordForm />
	</section>

	<section class="grid max-w-3xl gap-4">
		<div class="flex flex-wrap items-end justify-between gap-3">
			<div>
				<h2 class="text-sm font-semibold">{t.account.tokens}</h2>
				<p class="mt-1 text-[13px] text-muted-foreground">{t.account.tokensIntro}</p>
			</div>
			<Button
				variant="outline"
				onclick={() => {
					name = '';
					expiry = '90';
					created = '';
					open = true;
				}}
			>
				<Plus />{t.account.createToken}
			</Button>
		</div>
		{#if tokens.length === 0}
			<EmptyState icon={KeyRound} title={t.account.noTokens} />
		{:else}
			<ul class="divide-y rounded-lg border bg-card">
				{#each tokens as tk (tk.id)}
					<li class="flex flex-wrap items-center gap-3 px-3 py-2.5 sm:flex-nowrap">
						<div class="min-w-0 flex-1">
							<p class="flex items-center gap-2 font-medium">
								<span class="truncate">{tk.name}</span>
								{#if tk.revokedAt}<Badge>{t.common.revoked}</Badge>
								{:else if !active(tk)}<Badge tone="warning">{t.common.expired}</Badge>{/if}
							</p>
							<p class="text-xs text-muted-foreground">
								{t.common.created}
								{relativeTime(tk.createdAt)} · {t.common.expires}
								{tk.expiresAt ? absoluteTime(tk.expiresAt) : t.common.never} · {t.common.lastUsed}
								{tk.lastUsedAt ? relativeTime(tk.lastUsedAt) : t.common.never}
							</p>
						</div>
						{#if active(tk)}
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
	</section>
</div>

<Dialog bind:open title={t.account.createToken}>
	{#if created}
		<OneTimeSecret value={created} note={t.account.tokenCreated} />
	{:else}
		<form id="create-pat" class="grid gap-4" onsubmit={submit}>
			<Field id="pat-name" label={t.common.name}>
				<Input id="pat-name" bind:value={name} required maxlength={64} autocomplete="off" />
			</Field>
			<Field id="pat-expiry" label={t.common.expires}>
				<Select
					id="pat-expiry"
					bind:value={expiry}
					items={[
						{ value: '7', label: t.settings.expiry.d7 },
						{ value: '30', label: t.settings.expiry.d30 },
						{ value: '90', label: t.settings.expiry.d90 },
						{ value: '365', label: t.settings.expiry.d365 },
						{ value: '0', label: t.settings.expiry.never }
					]}
				/>
			</Field>
		</form>
	{/if}
	{#snippet footer()}
		{#if created}
			<Button onclick={() => (open = false)}>{t.common.close}</Button>
		{:else}
			<Button variant="outline" onclick={() => (open = false)}>{t.common.cancel}</Button>
			<Button type="submit" form="create-pat" disabled={busy || !name.trim()}>
				{busy ? t.common.creating : t.common.create}
			</Button>
		{/if}
	{/snippet}
</Dialog>

<ConfirmDialog
	bind:open={revokeOpen}
	title={t.account.revokeTitle(toRevoke?.name ?? '')}
	confirmLabel={t.common.revoke}
	onconfirm={revoke}
/>
