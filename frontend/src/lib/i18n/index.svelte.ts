import zhCN from './locales/zh-CN.json';
import en from './locales/en.json';

const messages: Record<string, any> = { 'zh-CN': zhCN, en };

let _locale = $state<string>('zh-CN');

// Initialize from localStorage
if (typeof window !== 'undefined') {
	const saved = localStorage.getItem('locale');
	const nav = navigator.language;
	_locale = saved || (nav.startsWith('en') ? 'en' : 'zh-CN');
}

export function getLocale(): string {
	return _locale;
}

export function setLocale(lang: string) {
	_locale = lang;
	if (typeof window !== 'undefined') {
		localStorage.setItem('locale', lang);
	}
}

function resolveKey(obj: any, path: string): string {
	let val = obj;
	for (const k of path.split('.')) {
		val = val?.[k];
	}
	return typeof val === 'string' ? val : path;
}

// Simple translation function - reads _locale reactively
export function t(key: string): string {
	return resolveKey(messages[_locale] ?? messages['zh-CN'], key);
}
