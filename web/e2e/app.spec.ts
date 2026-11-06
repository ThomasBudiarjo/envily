import { expect, test, type Browser, type Page } from '@playwright/test';

// Sign-ins are rate limited (5 per minute per IP), so the admin signs in
// once and later tests reuse that session.
const adminState = 'test-results/admin-state.json';

const admin = { name: 'Ada Admin', email: 'ada@example.com', password: 'correct horse battery' };

async function signIn(page: Page, email = admin.email, password = admin.password) {
	await page.goto('/login');
	await page.fill('#email', email);
	await page.fill('#password', password);
	await page.click('button[type=submit]');
	await expect(page).not.toHaveURL(/\/login/);
}

async function asAdmin(browser: Browser): Promise<Page> {
	const ctx = await browser.newContext({ storageState: adminState });
	return ctx.newPage();
}

function watchRequests(page: Page) {
	const bad: string[] = [];
	page.on('request', (r) => {
		const u = new URL(r.url());
		if (!['http:', 'https:'].includes(u.protocol)) return;
		if (
			u.host !== new URL(page.url() || 'http://localhost').host &&
			!u.hostname.startsWith('localhost')
		)
			bad.push(r.url());
	});
	page.on('console', (m) => {
		if (m.type() === 'error' && m.text().includes('Content Security Policy')) bad.push(m.text());
	});
	return bad;
}

test.describe.configure({ mode: 'serial' });

test('first run: setup creates the admin and signs in', async ({ page }) => {
	await page.goto('/');
	await expect(page).toHaveURL(/\/setup$/);
	await page.fill('#name', admin.name);
	await page.fill('#email', admin.email);
	await page.fill('#password', 'short');
	await page.click('button[type=submit]');
	await expect(page.getByText('Must be at least 12 characters.')).toBeVisible();
	await page.fill('#password', admin.password);
	await page.click('button[type=submit]');
	await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible();
	// Setup is one-shot.
	await page.goto('/setup');
	await expect(page).not.toHaveURL(/\/setup/);
});

test('wrong password is rejected; next= is honoured only for local paths', async ({ page }) => {
	await page.goto('/login');
	await page.fill('#email', admin.email);
	await page.fill('#password', 'not the password');
	await page.click('button[type=submit]');
	await expect(page.getByRole('alert')).toHaveText('Email or password is incorrect.');

	await page.goto('/login?next=//evil.example/x');
	await page.fill('#email', admin.email);
	await page.fill('#password', admin.password);
	await page.click('button[type=submit]');
	await expect(page).toHaveURL(/localhost:\d+\/$/);
	await page.context().storageState({ path: adminState });
});

test('secrets: draft, save, reveal, edit with conflict, history', async ({ browser }) => {
	const page = await asAdmin(browser);
	const bad = watchRequests(page);
	await page.goto('/');
	await page.getByRole('button', { name: 'New project' }).first().click();
	await page.fill('#project-name', 'Web App');
	await expect(page.locator('#project-slug')).toHaveValue('web-app');
	await page.getByRole('button', { name: 'Create', exact: true }).click();
	await expect(page).toHaveURL(/\/p\/web-app\/development$/);

	// Two new drafts, one with an invalid key that blocks saving.
	for (const [k, v] of [
		['API_KEY', 'sk_test_1'],
		['1BAD', 'x']
	]) {
		await page.getByRole('button', { name: 'Add secret' }).first().click();
		const row = page.locator('[data-testid=secret-row][data-state=new]').first();
		await row.locator('input').fill(k);
		await row.locator('textarea').fill(v);
	}
	await expect(page.getByTestId('save-bar')).toContainText('2 unsaved changes');
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await expect(page.locator('[data-key="1BAD"]').getByRole('alert')).toBeVisible();
	await page.locator('[data-key="1BAD"]').getByRole('button', { name: 'Remove' }).click();
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await expect(page.getByTestId('save-bar')).toHaveCount(0);
	await expect(page.getByTestId('secret-count')).toHaveText('1 secret');

	// Values are masked until revealed.
	const row = page.locator('[data-testid=secret-row][data-key=API_KEY]');
	await expect(row.getByTestId('secret-value')).toHaveText('••••••••');
	await row.getByRole('button', { name: 'Reveal value' }).click();
	await expect(row.getByTestId('secret-value')).toHaveText('sk_test_1');

	// Someone else changes the secret while we edit it: saving reports a conflict.
	const other = await asAdmin(browser);
	await other.goto('/');
	const status = await other.evaluate(async () => {
		const r = await fetch('/api/v1/projects/web-app/environments/development/secrets/API_KEY', {
			method: 'PUT',
			headers: { 'Content-Type': 'application/json', 'X-Envily-Client': 'web' },
			body: JSON.stringify({ value: 'sk_test_theirs' })
		});
		return r.status;
	});
	expect(status).toBe(200);
	await other.close();

	await row.getByTestId('secret-value').click();
	await row.locator('textarea').fill('sk_test_mine');
	await row.locator('textarea').blur();
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await expect(page.getByTestId('conflicts')).toContainText('API_KEY');
	await page.getByRole('button', { name: 'Reload secrets' }).click();
	await expect(page.getByTestId('save-bar')).toHaveCount(0);
	await row.getByRole('button', { name: 'Reveal value' }).click();
	await expect(row.getByTestId('secret-value')).toHaveText('sk_test_theirs');

	// History lists both versions and restores the first.
	await page.getByRole('button', { name: 'Actions: API_KEY' }).click();
	await page.getByRole('menuitem', { name: 'Version history' }).click();
	await expect(page.getByText('Version 2', { exact: true })).toBeVisible();
	await page.getByRole('button', { name: 'Restore this version' }).click();
	await page.getByRole('alertdialog').getByRole('button', { name: 'Restore this version' }).click();
	await expect(page.getByText('Version 3', { exact: true })).toBeVisible();
	await page.getByRole('dialog').getByRole('button', { name: 'Close' }).click();
	await row.getByRole('button', { name: 'Reveal value' }).click();
	await expect(row.getByTestId('secret-value')).toHaveText('sk_test_1');

	expect(bad).toEqual([]);
});

test('import previews changes and applies them', async ({ browser }) => {
	const page = await asAdmin(browser);
	await page.goto('/p/web-app/staging');
	await page.getByRole('button', { name: 'Import from .env' }).click();
	await page.fill(
		'#import-text',
		'DATABASE_URL="postgres://db"\nexport REDIS_URL=redis://r # cache\n'
	);
	await expect(page.getByTestId('import-preview')).toContainText('Added (2)');
	await page.getByRole('dialog').getByRole('button', { name: 'Import', exact: true }).click();
	await expect(page.getByTestId('secret-count')).toHaveText('2 secrets');

	await page.goto('/p/web-app/compare');
	const apiRow = page.getByTestId('compare-table').locator('tr', { hasText: 'API_KEY' });
	await expect(apiRow).toHaveAttribute('data-gap', 'true');
});

test('viewer sees a read-only table; temporary password forces a change', async ({ browser }) => {
	const page = await asAdmin(browser);
	await page.goto('/admin/users');
	await page.getByRole('button', { name: 'Add user' }).click();
	await page.fill('#user-name', 'Vera Viewer');
	await page.fill('#user-email', 'vera@example.com');
	await page.getByRole('dialog').getByRole('button', { name: 'Create' }).click();
	const temp = (await page.getByTestId('one-time-secret').textContent())!.trim();
	expect(temp.length).toBeGreaterThanOrEqual(12);

	await page.goto('/p/web-app/settings?tab=members');
	await page.getByRole('button', { name: 'Add member' }).click();
	await page.locator('#member-user').click();
	await page.locator('#member-user').pressSequentially('vera');
	await page.getByRole('option', { name: /Vera Viewer/ }).click();
	await page.locator('#member-role').click();
	await page.getByRole('option', { name: 'Viewer' }).click();
	await page.getByRole('dialog').getByRole('button', { name: 'Add member' }).click();
	await expect(page.getByTestId('member-list')).toContainText('vera@example.com');

	await page.context().clearCookies();
	await signIn(page, 'vera@example.com', temp);
	await expect(page).toHaveURL(/\/change-password$/);
	await page.fill('#current-password', temp);
	await page.fill('#new-password', 'vera has a new password');
	await page.fill('#confirm-password', 'vera has a new password');
	await page.getByRole('button', { name: 'Change password' }).click();
	await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible();

	await page.goto('/p/web-app/development');
	await expect(page.getByText('You have read-only access to this environment.')).toBeVisible();
	await expect(page.getByRole('button', { name: 'Add secret' })).toHaveCount(0);
	await expect(page.getByRole('link', { name: 'Audit log' })).toHaveCount(0);
});

test('theme toggle persists across reloads', async ({ browser }) => {
	const page = await asAdmin(browser);
	await page.goto('/');
	const isDark = () => page.evaluate(() => document.documentElement.classList.contains('dark'));
	const start = await isDark();
	// light → dark → system cycle: click until the class flips.
	for (let i = 0; i < 3 && (await isDark()) === start; i++)
		await page.getByTestId('theme-toggle').click();
	const flipped = await isDark();
	expect(flipped).not.toBe(start);
	await page.reload();
	expect(await isDark()).toBe(flipped);
});
