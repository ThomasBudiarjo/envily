import { defineConfig, devices } from '@playwright/test';

const port = Number(process.env.E2E_PORT ?? 18091);

// Runs against the real binary (make e2e builds it first). Tests share one
// fresh instance and run in order: the first test performs the setup.
export default defineConfig({
	testDir: 'e2e',
	fullyParallel: false,
	workers: 1,
	retries: 0,
	reporter: process.env.CI ? 'github' : 'list',
	use: { baseURL: `http://localhost:${port}`, trace: 'retain-on-failure' },
	projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
	webServer: {
		command: 'sh e2e/server.sh',
		// 127.0.0.1, not localhost: ::1 connections can hang instead of failing.
		url: `http://127.0.0.1:${port}/readyz`,
		env: { E2E_PORT: String(port) },
		reuseExistingServer: false,
		stdout: 'ignore',
		stderr: 'pipe',
		timeout: 20_000
	}
});
