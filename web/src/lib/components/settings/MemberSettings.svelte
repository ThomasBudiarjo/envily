<script lang="ts">
	import { goto, invalidate } from '$app/navigation';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import UserPlus from '@lucide/svelte/icons/user-plus';
	import { toast } from 'svelte-sonner';
	import { addMember, listMembers, removeMember, updateMember } from '#lib/api/projects.js';
	import type { Member, Project, Role, User } from '#lib/api/types.js';
	import { listUsers } from '#lib/api/users.js';
	import Button from '#lib/components/ui/Button.svelte';
	import Combobox from '#lib/components/ui/Combobox.svelte';
	import ConfirmDialog from '#lib/components/ui/ConfirmDialog.svelte';
	import Dialog from '#lib/components/ui/Dialog.svelte';
	import Field from '#lib/components/ui/Field.svelte';
	import IconButton from '#lib/components/ui/IconButton.svelte';
	import Input from '#lib/components/ui/Input.svelte';
	import Select from '#lib/components/ui/Select.svelte';
	import { errorMessage } from '#lib/forms.js';
	import { t } from '#lib/i18n/index.js';
	import { session } from '#lib/stores/session.svelte.js';

	let { project, isOwner }: { project: Project; isOwner: boolean } = $props();

	const roles: { value: Role; label: string }[] = [
		{ value: 'owner', label: t.projects.role.owner },
		{ value: 'editor', label: t.projects.role.editor },
		{ value: 'viewer', label: t.projects.role.viewer }
	];

	let members = $state<Member[]>([]);
	let users = $state<User[]>([]);
	let adding = $state(false);
	let pickUser = $state('');
	let email = $state('');
	let role = $state<Role>('editor');
	let busy = $state(false);
	let toRemove = $state<Member | null>(null);
	let removeOpen = $state(false);

	async function load() {
		try {
			members = await listMembers(project.slug);
		} catch (err) {
			toast.error(errorMessage(err));
		}
	}

	$effect(() => {
		void project.slug;
		void load();
	});

	async function openAdd() {
		pickUser = email = '';
		role = 'editor';
		adding = true;
		if (session.isAdmin && users.length === 0) users = await listUsers().catch(() => []);
	}

	const candidates = $derived(
		users
			.filter((u) => !u.disabledAt && !members.some((m) => m.userId === u.id))
			.map((u) => ({ value: u.id, label: u.name || u.email, hint: u.email }))
	);

	async function submitAdd(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		try {
			await addMember(
				project.slug,
				session.isAdmin ? { userId: pickUser, role } : { email: email.trim(), role }
			);
			adding = false;
			await load();
		} catch (err) {
			toast.error(errorMessage(err));
		} finally {
			busy = false;
		}
	}

	async function addSelf() {
		try {
			await addMember(project.slug, { userId: session.user!.id, role: 'owner' });
			await Promise.all([invalidate('app:project'), invalidate('app:projects')]);
			await load();
		} catch (err) {
			toast.error(errorMessage(err));
		}
	}

	async function changeRole(m: Member, next: string) {
		if (next === m.role) return;
		try {
			await updateMember(project.slug, m.userId, next as Role);
			if (m.userId === session.user?.id) await invalidate('app:project');
		} catch (err) {
			toast.error(errorMessage(err));
		}
		await load();
	}

	async function remove() {
		if (!toRemove) return;
		const self = toRemove.userId === session.user?.id;
		try {
			await removeMember(project.slug, toRemove.userId);
		} catch (err) {
			toast.error(errorMessage(err));
			throw err;
		}
		if (self) {
			await invalidate('app:projects');
			await goto('/', { replace: true });
		} else await load();
	}
</script>

<div class="grid max-w-3xl gap-4">
	{#if project.role === null && session.isAdmin}
		<div
			class="flex flex-wrap items-center justify-between gap-3 rounded-lg border bg-muted/40 p-3 text-[13px]"
		>
			<span>{t.projects.role.admin}</span>
			<Button size="sm" onclick={addSelf}>{t.settings.addSelf}</Button>
		</div>
	{/if}
	{#if isOwner}
		<div><Button onclick={openAdd}><UserPlus />{t.settings.addMember}</Button></div>
	{/if}
	<ul class="divide-y rounded-lg border bg-card" data-testid="member-list">
		{#each members as m (m.userId)}
			{@const self = m.userId === session.user?.id}
			<li class="flex flex-wrap items-center gap-3 px-3 py-2.5 sm:flex-nowrap">
				<div class="min-w-0 flex-1">
					<p class="truncate font-medium">
						{m.name || m.email}
						{#if self}<span class="text-xs font-normal text-muted-foreground">({t.common.you})</span
							>{/if}
					</p>
					<p class="truncate text-xs text-muted-foreground">{m.email}</p>
				</div>
				{#if isOwner}
					<Select
						value={m.role}
						items={roles}
						label="{t.settings.role}: {m.email}"
						class="h-8 w-32"
						onchange={(v) => changeRole(m, v)}
					/>
				{:else}
					<span class="text-[13px] text-muted-foreground">{t.projects.role[m.role]}</span>
				{/if}
				{#if isOwner || self}
					<IconButton
						small
						label={self ? t.settings.leaveProject : `${t.settings.removeMember}: ${m.email}`}
						class="text-destructive"
						onclick={() => {
							toRemove = m;
							removeOpen = true;
						}}
					>
						<Trash2 />
					</IconButton>
				{/if}
			</li>
		{/each}
	</ul>
</div>

<Dialog bind:open={adding} title={t.settings.addMember}>
	<form id="add-member" class="grid gap-4" onsubmit={submitAdd}>
		{#if session.isAdmin}
			<Field id="member-user" label={t.settings.member}>
				<Combobox
					id="member-user"
					bind:value={pickUser}
					items={candidates}
					placeholder={t.settings.pickUser}
					empty={t.command.empty}
				/>
			</Field>
		{:else}
			<Field id="member-email" label={t.settings.memberEmail}>
				<Input id="member-email" type="email" bind:value={email} required />
			</Field>
		{/if}
		<Field id="member-role" label={t.settings.role}>
			<Select id="member-role" bind:value={role} items={roles} />
		</Field>
	</form>
	{#snippet footer()}
		<Button variant="outline" onclick={() => (adding = false)}>{t.common.cancel}</Button>
		<Button
			type="submit"
			form="add-member"
			disabled={busy || (session.isAdmin ? !pickUser : !email.trim())}
		>
			{t.settings.addMember}
		</Button>
	{/snippet}
</Dialog>

<ConfirmDialog
	bind:open={removeOpen}
	title={toRemove?.userId === session.user?.id
		? t.settings.leaveProject
		: t.settings.removeMemberTitle(toRemove?.name || toRemove?.email || '')}
	confirmLabel={toRemove?.userId === session.user?.id ? t.settings.leaveProject : t.common.remove}
	onconfirm={remove}
/>
