import { test, expect, type Page } from '@playwright/test';

const USER = 'admin';
const PASS = 'admin123';

async function login(page: Page) {
	await page.goto('/login');
	await page.fill('input[id="username"]', USER);
	await page.fill('input[id="password"]', PASS);
	await page.click('button[type="submit"]');
	await page.waitForURL('**/overview', { timeout: 15000 });
}

const authedPages = [
	{ path: '/overview', contains: ['概览', '容器', 'Compose'] },
	{ path: '/hosts', contains: ['主机'] },
	{ path: '/containers', contains: ['容器'] },
	{ path: '/compose', contains: ['Compose'] },
	{ path: '/browse', contains: ['文件'] },
	{ path: '/settings', contains: ['设置'] },
];

test.describe('All pages load without crash', () => {
	test('login page loads', async ({ page }) => {
		const errors: string[] = [];
		page.on('pageerror', e => errors.push(e.message));
		await page.goto('/login');
		await page.waitForSelector('input[id="username"]', { timeout: 10000 });
		expect(await page.title()).toBeTruthy();
		expect(errors).toHaveLength(0);
	});

	test('login works and redirects to overview', async ({ page }) => {
		await page.goto('/login');
		await page.fill('input[id="username"]', USER);
		await page.fill('input[id="password"]', PASS);
		await page.click('button[type="submit"]');
		await page.waitForURL('**/overview', { timeout: 15000 });
		const body = await page.textContent('body');
		expect(body).toContain('概览');
	});

	for (const { path, contains } of authedPages) {
		test(`${path} loads after login`, async ({ page }) => {
			const errors: string[] = [];
			page.on('pageerror', e => errors.push(e.message));
			await login(page);
			await page.goto(path);
			await page.waitForLoadState('networkidle', { timeout: 10000 });
			const body = await page.textContent('body');
			for (const c of contains) {
				expect(body).toContain(c);
			}
			expect(errors).toHaveLength(0);
		});
	}

	test('home page redirects to overview when authed', async ({ page }) => {
		await login(page);
		await page.goto('/');
		await page.waitForURL('**/overview', { timeout: 15000 });
	});

	test('404 page shows error', async ({ page }) => {
		await login(page);
		await page.goto('/nonexistent-route', { waitUntil: 'networkidle' });
		const body = await page.textContent('body');
		expect(body).toBeTruthy();
	});
});
