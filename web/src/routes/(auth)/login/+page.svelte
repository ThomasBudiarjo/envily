<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { login } from '#lib/api/auth.js';
	import { ApiError } from '#lib/api/client.js';
	import Button from '#lib/components/ui/Button.svelte';
	import Field from '#lib/components/ui/Field.svelte';
	import Input from '#lib/components/ui/Input.svelte';
	import { errorMessage } from '#lib/forms.js';
	import { t } from '#lib/i18n/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { safeNext } from '#lib/utils.js';

	let email = $state('');
	let password = $state('');
	let error = $state('');
	let busy = $state(false);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		error = '';
		try {
			await login(email, password);
			await session.load();
			await goto(safeNext(page.url.searchParams.get('next')), { replace: true });
		} catch (err) {
			error =
				err instanceof ApiError && err.code === 'invalid_credentials'
					? t.auth.invalid
					: errorMessage(err);
			password = '';
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>{t.auth.login} · {t.app.name}</title></svelte:head>

<h1 class="mb-5 text-base font-semibold">{t.auth.loginTitle}</h1>
<form class="grid gap-4" onsubmit={submit}>
	<Field id="email" label={t.common.email}>
		<Input id="email" type="email" bind:value={email} autocomplete="username" required autofocus />
	</Field>
	<Field id="password" label={t.common.password}>
		<Input
			id="password"
			type="password"
			bind:value={password}
			autocomplete="current-password"
			required
		/>
	</Field>
	{#if error}
		<p role="alert" class="rounded-md bg-destructive/10 px-3 py-2 text-[13px] text-destructive">
			{error}
		</p>
	{/if}
	<Button type="submit" disabled={busy}>{busy ? t.auth.loggingIn : t.auth.login}</Button>
</form>
