import { getMe, getSetupStatus } from '#lib/api/auth.js';
import { ApiError } from '#lib/api/client.js';
import type { Me } from '#lib/api/types.js';

// The signed-in user, loaded once by the root layout.
class Session {
	me = $state<Me | null>(null);
	loaded = $state(false);
	needsSetup = $state(false);

	get user() {
		return this.me?.user ?? null;
	}

	get isAdmin() {
		return this.me?.isAdmin ?? false;
	}

	/** Loads /auth/me; a 401 leaves the session empty. */
	async load(): Promise<Me | null> {
		if (!this.loaded) this.needsSetup = (await getSetupStatus()).needsSetup;
		this.loaded = true;
		if (this.needsSetup) return (this.me = null);
		try {
			this.me = await getMe();
		} catch (e) {
			if (!(e instanceof ApiError) || e.status !== 401) throw e;
			this.me = null;
		}
		this.loaded = true;
		return this.me;
	}

	clear() {
		this.me = null;
	}
}

export const session = new Session();
