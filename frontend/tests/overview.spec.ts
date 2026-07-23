import { test, expect } from '@playwright/test';

const BASE = 'http://192.168.132.86:8080';

test('概览页打开不卡死', async ({ page }) => {
	// 先登录
	await page.goto(BASE + '/login');
	await page.fill('input[id="username"]', 'admin');
	await page.fill('input[id="password"]', 'admin123');
	await page.click('button[type="submit"]');
	await page.waitForTimeout(2000);

	// 导航到概览页
	await page.goto(BASE + '/overview');
	await page.waitForTimeout(3000);

	// 检查页面有内容，没有崩溃
	const body = await page.textContent('body');
	expect(body).toContain('概览');
	expect(body).toContain('容器');
	expect(body).toContain('Compose');

	// 检查没有崩溃（控制台无错误）
	const errors: string[] = [];
	page.on('pageerror', err => errors.push(err.message));
	await page.waitForTimeout(1000);
	expect(errors.length).toBe(0);
});
