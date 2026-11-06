/// <reference types="vitest/config" />
import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({
			compilerOptions: {
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},
			// Static SPA embedded in the Go binary: every route falls back to index.html.
			adapter: adapter({ pages: 'build', assets: 'build', fallback: 'index.html' }),
			csp: {
				mode: 'hash',
				directives: {
					'default-src': ['self'],
					'script-src': ['self'],
					'style-src': ['self', 'unsafe-inline'],
					'img-src': ['self', 'data:'],
					'font-src': ['self'],
					'connect-src': ['self'],
					'base-uri': ['self'],
					'form-action': ['self']
				}
			}
		})
	],
	server: {
		port: 5173,
		strictPort: true,
		proxy: {
			'/api': 'http://localhost:8080',
			'/healthz': 'http://localhost:8080'
		}
	},
	test: {
		include: ['src/**/*.test.ts'],
		passWithNoTests: true
	}
});
