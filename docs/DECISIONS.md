# Decisions

Choices made during implementation where the plan was silent, ambiguous, or
conflicted with the tools as they exist today. Newest last.

## SvelteKit 3 has no `svelte.config.js`

The plan lists `web/svelte.config.js`. SvelteKit 3 (current stable) no longer
reads that file: configuration is passed to the `sveltekit()` Vite plugin in
`web/vite.config.ts`, and the `$lib` alias is now `#lib` (a package.json
`imports` entry). The adapter, SPA fallback, and CSP settings live there.

## `go.mod` targets Go 1.26

The latest stable Go is 1.27, but `go.mod` declares `go 1.26.0` so the module
builds with any supported toolchain (including the one on the development
machine, which pins `GOTOOLCHAIN=local`). CI, Docker, and releases build with
the latest stable Go.

## `ENVILY_PUBLIC_URL` is always required

The plan says it is required "in production". The server cannot tell
production from development, and the Origin check and the cookie `Secure`
attribute both depend on it, so it is required everywhere. `make dev` sets it
to the Vite dev server URL.

## Placeholder for an unbuilt UI

`go:embed` needs at least one file in `web/build`. Instead of committing a
placeholder `index.html` (which every UI build would overwrite and leave as a
modified tracked file), the repository tracks `web/build/.gitkeep` and the Go
SPA handler serves a built-in placeholder page when `index.html` is missing.

## Access logs never include query strings

The plan asks to omit query strings for secret paths. The access log omits
them for every path, which is simpler and strictly safer.
