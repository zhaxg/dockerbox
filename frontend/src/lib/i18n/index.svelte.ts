import { writable, derived, get } from 'svelte/store';
import zhCN from './locales/zh-CN.json';
import en from './locales/en.json';

const messages: Record<string, any> = { 'zh-CN': zhCN, en };

// Use writable store - Svelte 5 still supports store reactivity in templates
function getInitialLocale(): string {
	if (typeof window === 'undefined') return 'zh-CN';
	const saved = localStorage.getItem('locale');
	if (saved && messages[saved]) return saved;
	const nav = navigator.language;
	return nav.startsWith('en') ? 'en' : 'zh-CN';
}

export const locale = writable<string>(getInitialLocale());

export function setLocale(lang: string) {
	locale.set(lang);
	if (typeof window !== 'undefined') {
		localStorage.setItem('locale', lang);
	}
}

export function getLocale(): string {
	return get(locale);
}

function resolveKey(obj: any, path: string): string {
	let val = obj;
	for (const k of path.split('.')) {
		val = val?.[k];
	}
	return typeof val === 'string' ? val : path;
}

// Translation function - returns a Svelte store that auto-updates
// Usage in template: {$_t('key')}  (import as $_t)
function createTranslator() {
	return derived(locale, ($locale) => {
		return (key: string): string => {
			return resolveKey(messages[$locale] ?? messages['zh-CN'], key);
		};
	});
}

export const _t = createTranslator();
