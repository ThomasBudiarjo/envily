// Light, dark, or follow the system. The choice is kept in localStorage;
// "system" tracks prefers-color-scheme live.

export type ThemeChoice = 'light' | 'dark' | 'system';

const KEY = 'envily-theme';

function read(): ThemeChoice {
	const v = typeof localStorage === 'undefined' ? null : localStorage.getItem(KEY);
	return v === 'light' || v === 'dark' ? v : 'system';
}

class Theme {
	choice = $state<ThemeChoice>(read());
	#media = typeof matchMedia === 'undefined' ? null : matchMedia('(prefers-color-scheme: dark)');
	#systemDark = $state(this.#media?.matches ?? false);

	constructor() {
		this.#media?.addEventListener('change', (e) => {
			this.#systemDark = e.matches;
			this.apply();
		});
		this.apply();
	}

	get dark() {
		return this.choice === 'dark' || (this.choice === 'system' && this.#systemDark);
	}

	set(choice: ThemeChoice) {
		this.choice = choice;
		if (choice === 'system') localStorage.removeItem(KEY);
		else localStorage.setItem(KEY, choice);
		this.apply();
	}

	/** Cycles light → dark → system. */
	toggle() {
		this.set(this.choice === 'light' ? 'dark' : this.choice === 'dark' ? 'system' : 'light');
	}

	apply() {
		if (typeof document !== 'undefined')
			document.documentElement.classList.toggle('dark', this.dark);
	}
}

export const theme = new Theme();
