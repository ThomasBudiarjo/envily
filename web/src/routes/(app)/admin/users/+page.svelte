<script lang="ts">
	import Ellipsis from '@lucide/svelte/icons/ellipsis';
	import UserPlus from '@lucide/svelte/icons/user-plus';
	import { toast } from 'svelte-sonner';
	import type { User } from '#lib/api/types.js';
	import { createUser, listUsers, updateUser } from '#lib/api/users.js';
	import Badge from '#lib/components/ui/Badge.svelte';
	import Button from '#lib/components/ui/Button.svelte';
	import Checkbox from '#lib/components/ui/Checkbox.svelte';
	import ConfirmDialog from '#lib/components/ui/ConfirmDialog.svelte';
	import Dialog from '#lib/components/ui/Dialog.svelte';
	import DropdownMenu from '#lib/components/ui/DropdownMenu.svelte';
	import Field from '#lib/components/ui/Field.svelte';
	import Input from '#lib/components/ui/Input.svelte';
	import Label from '#lib/components/ui/Label.svelte';
	import OneTimeSecret from '#lib/components/ui/OneTimeSecret.svelte';
	import { errorMessage, fieldErrors } from '#lib/forms.js';
	import { t } from '#lib/i18n/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { relativeTime } from '#lib/utils.js';

	let users = $state<User[]>([]);
	let adding = $state(false);
	let name = $state('');
	let email = $state('');
	let isAdmin = $state(false);
	let errors = $state<Record<string, string>>({});
	let busy = $state(false);
	let tempFor = $state('');
	let tempPassword = $state('');
	let tempOpen = $state(false);
	let confirm = $state<{ user: User; kind: 'reset' | 'disable' } | null>(null);
	let confirmOpen = $state(false);

	async function load() {
		try {
			users = await listUsers();
		} catch (err) {
			toast.error(errorMessage(err));
		}
	}
	$effect(() => void load());

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errors = {};
		busy = true;
		try {
			const r = await createUser({ name: name.trim(), email: email.trim(), isAdmin });
			adding = false;
			showTemp(r.user, r.temporaryPassword);
			await load();
		} catch (err) {
			errors = fieldErrors(err);
			if (!Object.keys(errors).length) toast.error(errorMessage(err));
		} finally {
			busy = false;
		}
	}

	function showTemp(u: User, password: string) {
		tempFor = u.email;
		tempPassword = password;
		tempOpen = true;
	}

	async function patch(u: User, body: Parameters<typeof updateUser>[1]) {
		try {
			const r = await updateUser(u.id, body);
			if (r.temporaryPassword) showTemp(r.user, r.temporaryPassword);
			await load();
		} catch (err) {
			toast.error(errorMessage(err));
			throw err;
		}
	}

	function ask(user: User, kind: 'reset' | 'disable') {
		confirm = { user, kind };
		confirmOpen = true;
	}

	// Never keep a temporary password around after its dialog closes.
	$effect(() => {
		if (!tempOpen) tempPassword = '';
	});
</script>

<svelte:head><title>{t.users.title} · {t.app.name}</title></svelte:head>

<div class="mb-6 flex items-center justify-between gap-4">
	<h1 class="text-xl font-semibold tracking-tight">{t.users.title}</h1>
	<Button
		onclick={() => {
			name = email = '';
			isAdmin = false;
			errors = {};
			adding = true;
		}}
	>
		<UserPlus />{t.users.add}
	</Button>
</div>

<ul class="divide-y rounded-lg border bg-card" data-testid="user-list">
	{#each users as u (u.id)}
		{@const self = u.id === session.user?.id}
		<li class="flex items-center gap-3 px-3 py-2.5">
			<div class="min-w-0 flex-1">
				<p class="flex flex-wrap items-center gap-2 font-medium">
					<span class="truncate">{u.name || u.email}</span>
					{#if self}<span class="text-xs font-normal text-muted-foreground">({t.common.you})</span
						>{/if}
					{#if u.isAdmin}<Badge tone="accent">{t.nav.admin}</Badge>{/if}
					{#if u.disabledAt}<Badge tone="destructive">{t.users.disabled}</Badge>{/if}
					{#if u.mustChangePassword}<Badge tone="warning">{t.auth.changeTitle}</Badge>{/if}
				</p>
				<p class="truncate text-xs text-muted-foreground">
					{u.email} · {t.common.created}
					{relativeTime(u.createdAt)}
				</p>
			</div>
			{#if !self}
				<DropdownMenu
					items={[
						{
							label: u.isAdmin ? t.users.removeAdmin : t.users.makeAdmin,
							onselect: () => patch(u, { isAdmin: !u.isAdmin }).catch(() => {})
						},
						{ label: t.users.resetPassword, onselect: () => ask(u, 'reset') },
						u.disabledAt
							? {
									label: t.users.enable,
									onselect: () => patch(u, { disabled: false }).catch(() => {})
								}
							: { label: t.users.disable, onselect: () => ask(u, 'disable'), destructive: true }
					]}
				>
					{#snippet trigger({ props })}
						<button
							{...props}
							type="button"
							aria-label="{t.common.actions}: {u.email}"
							class="grid size-7 place-items-center rounded-md text-muted-foreground hover:bg-muted"
						>
							<Ellipsis class="size-4" />
						</button>
					{/snippet}
				</DropdownMenu>
			{/if}
		</li>
	{/each}
</ul>

<Dialog bind:open={adding} title={t.users.add}>
	<form id="add-user" class="grid gap-4" onsubmit={submit}>
		<Field id="user-name" label={t.common.name} error={errors.name}>
			<Input id="user-name" bind:value={name} required />
		</Field>
		<Field id="user-email" label={t.common.email} error={errors.email}>
			<Input id="user-email" type="email" bind:value={email} required invalid={!!errors.email} />
		</Field>
		<div class="flex items-center gap-2">
			<Checkbox id="user-admin" bind:checked={isAdmin} />
			<Label for="user-admin" class="font-normal">{t.users.isAdmin}</Label>
		</div>
	</form>
	{#snippet footer()}
		<Button variant="outline" onclick={() => (adding = false)}>{t.common.cancel}</Button>
		<Button type="submit" form="add-user" disabled={busy}
			>{busy ? t.common.creating : t.common.create}</Button
		>
	{/snippet}
</Dialog>

<Dialog bind:open={tempOpen} title={tempFor}>
	<OneTimeSecret value={tempPassword} note={t.users.tempPassword} />
	{#snippet footer()}
		<Button onclick={() => (tempOpen = false)}>{t.common.close}</Button>
	{/snippet}
</Dialog>

<ConfirmDialog
	bind:open={confirmOpen}
	title={confirm?.kind === 'reset'
		? t.users.resetTitle(confirm.user.name || confirm.user.email)
		: t.users.disableTitle(confirm?.user.name || confirm?.user.email || '')}
	description={confirm?.kind === 'reset' ? t.users.resetIntro : t.users.disableIntro}
	confirmLabel={confirm?.kind === 'reset' ? t.users.resetPassword : t.users.disable}
	onconfirm={() =>
		confirm &&
		patch(confirm.user, confirm.kind === 'reset' ? { resetPassword: true } : { disabled: true })}
/>
