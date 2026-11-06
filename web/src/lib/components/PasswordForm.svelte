<script lang="ts">
	import { toast } from 'svelte-sonner';
	import { changePassword } from '#lib/api/auth.js';
	import { ApiError } from '#lib/api/client.js';
	import Button from '#lib/components/ui/Button.svelte';
	import Field from '#lib/components/ui/Field.svelte';
	import Input from '#lib/components/ui/Input.svelte';
	import { errorMessage } from '#lib/forms.js';
	import { t } from '#lib/i18n/index.js';

	let { ondone }: { ondone?: () => void } = $props();

	let current = $state('');
	let next = $state('');
	let confirm = $state('');
	let errors = $state<Record<string, string>>({});
	let busy = $state(false);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errors = {};
		if (next.length < 12) errors.newPassword = t.auth.tooShort;
		else if (next !== confirm) errors.confirm = t.auth.mismatch;
		if (Object.keys(errors).length) return;
		busy = true;
		try {
			await changePassword(current, next);
			current = next = confirm = '';
			toast.success(t.auth.changed);
			ondone?.();
		} catch (err) {
			if (err instanceof ApiError && err.code === 'validation_failed') errors = err.details;
			else if (err instanceof ApiError && err.code === 'invalid_credentials')
				errors = { currentPassword: t.auth.invalid };
			else toast.error(errorMessage(err));
		} finally {
			busy = false;
		}
	}
</script>

<form class="grid gap-4" onsubmit={submit}>
	<Field id="current-password" label={t.auth.currentPassword} error={errors.currentPassword}>
		<Input
			id="current-password"
			type="password"
			bind:value={current}
			autocomplete="current-password"
			invalid={!!errors.currentPassword}
			required
		/>
	</Field>
	<Field
		id="new-password"
		label={t.auth.newPassword}
		hint={t.auth.passwordHint}
		error={errors.newPassword}
	>
		<Input
			id="new-password"
			type="password"
			bind:value={next}
			autocomplete="new-password"
			invalid={!!errors.newPassword}
			required
		/>
	</Field>
	<Field id="confirm-password" label={t.auth.confirmPassword} error={errors.confirm}>
		<Input
			id="confirm-password"
			type="password"
			bind:value={confirm}
			autocomplete="new-password"
			invalid={!!errors.confirm}
			required
		/>
	</Field>
	<div>
		<Button type="submit" disabled={busy}
			>{busy ? t.common.saving : t.account.changePassword}</Button
		>
	</div>
</form>
