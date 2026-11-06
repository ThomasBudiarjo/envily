<script lang="ts">
	import { goto } from '$app/navigation';
	import { setup } from '#lib/api/auth.js';
	import { ApiError } from '#lib/api/client.js';
	import Button from '#lib/components/ui/Button.svelte';
	import Field from '#lib/components/ui/Field.svelte';
	import Input from '#lib/components/ui/Input.svelte';
	import { errorMessage, fieldErrors } from '#lib/forms.js';
	import { t } from '#lib/i18n/index.js';
	import { session } from '#lib/stores/session.svelte.js';

	let name = $state('');
	let email = $state('');
	let password = $state('');
	let setupToken = $state('');
	let errors = $state<Record<string, string>>({});
	let error = $state('');
	let busy = $state(false);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errors = {};
		error = '';
		if (password.length < 12) {
			errors = { password: t.auth.tooShort };
			return;
		}
		busy = true;
		try {
			await setup({ name, email, password, setupToken: setupToken || undefined });
			session.loaded = false;
			await session.load();
			await goto('/', { replace: true });
		} catch (err) {
			if (err instanceof ApiError && err.status === 409) {
				session.loaded = false;
				await goto('/login', { replace: true });
				return;
			}
			errors = fieldErrors(err);
			if (!Object.keys(errors).length) error = errorMessage(err);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>{t.auth.setupTitle} · {t.app.name}</title></svelte:head>

<h1 class="text-base font-semibold">{t.auth.setupTitle}</h1>
<p class="mt-1 mb-5 text-[13px] text-muted-foreground">{t.auth.setupIntro}</p>
<form class="grid gap-4" onsubmit={submit}>
	<Field id="name" label={t.common.name} error={errors.name}>
		<Input id="name" bind:value={name} autocomplete="name" required autofocus />
	</Field>
	<Field id="email" label={t.common.email} error={errors.email}>
		<Input id="email" type="email" bind:value={email} autocomplete="username" required />
	</Field>
	<Field id="password" label={t.common.password} hint={t.auth.passwordHint} error={errors.password}>
		<Input
			id="password"
			type="password"
			bind:value={password}
			autocomplete="new-password"
			invalid={!!errors.password}
			required
		/>
	</Field>
	<Field
		id="setup-token"
		label="{t.auth.setupToken} ({t.common.optional.toLowerCase()})"
		hint={t.auth.setupTokenHint}
		error={errors.setupToken}
	>
		<Input id="setup-token" bind:value={setupToken} autocomplete="off" class="font-mono" />
	</Field>
	{#if error}
		<p role="alert" class="rounded-md bg-destructive/10 px-3 py-2 text-[13px] text-destructive">
			{error}
		</p>
	{/if}
	<Button type="submit" disabled={busy}>{busy ? t.common.creating : t.auth.setupSubmit}</Button>
</form>
