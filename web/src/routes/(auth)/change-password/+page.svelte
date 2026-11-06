<script lang="ts">
	import { goto } from '$app/navigation';
	import { logout } from '#lib/api/auth.js';
	import PasswordForm from '#lib/components/PasswordForm.svelte';
	import { t } from '#lib/i18n/index.js';
	import { session } from '#lib/stores/session.svelte.js';

	async function done() {
		await session.load();
		await goto('/', { replace: true });
	}

	async function signOut() {
		await logout().catch(() => {});
		session.clear();
		await goto('/login', { replace: true });
	}
</script>

<svelte:head><title>{t.auth.changeTitle} · {t.app.name}</title></svelte:head>

<h1 class="text-base font-semibold">{t.auth.changeTitle}</h1>
{#if session.me?.mustChangePassword}
	<p class="mt-1 text-[13px] text-muted-foreground">{t.auth.changeIntro}</p>
{/if}
<div class="mt-5"><PasswordForm ondone={done} /></div>
<button
	type="button"
	class="mt-4 text-[13px] text-muted-foreground underline-offset-4 hover:underline"
	onclick={signOut}
>
	{t.nav.logout}
</button>
