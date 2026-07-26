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

test('check toast after file deletion', async ({ page }) => {
	await login(page);
	
	// Navigate to browse
	await page.goto('/browse');
	await page.waitForLoadState('networkidle', { timeout: 15000 });
	
	// Click the first available root mount point
	const roots = page.locator('button:has-text("tmp"), button:has-text("home"), button:has-text("data"), button:has-text("etc")');
	const rootCount = await roots.count();
	console.log(`Found ${rootCount} root buttons`);
	
	if (rootCount === 0) {
		// Maybe already in a directory
		console.log('No root buttons found, checking current state');
		const bodyText = await page.textContent('body');
		console.log('Body text snippet:', bodyText?.substring(0, 500));
		return;
	}
	
	await roots.first().click();
	await page.waitForLoadState('networkidle', { timeout: 10000 });
	
	// Wait for file list to load
	await page.waitForTimeout(2000);
	
	// Check what files exist
	const rows = page.locator('tr[data-file-row="true"]');
	const rowCount = await rows.count();
	console.log(`Found ${rowCount} file rows`);
	
	if (rowCount === 0) {
		console.log('No files to delete');
		return;
	}
	
	// Click first file to select it
	await rows.first().click();
	await page.waitForTimeout(500);
	
	// Right-click to open context menu
	await rows.first().click({ button: 'right' });
	await page.waitForTimeout(500);
	
	// Screenshot to see context menu
	await page.screenshot({ path: '.playwright-mcp/01-context-menu.png' });
	
	// Click Delete option
	const deleteOption = page.locator('[data-context-item="delete"], button:has-text("Delete"), div:has-text("删除")').first();
	const deleteVisible = await deleteOption.isVisible({ timeout: 3000 }).catch(() => false);
	console.log(`Delete option visible: ${deleteVisible}`);
	
	if (!deleteVisible) {
		// List all visible context menu items
		const menuItems = page.locator('[role="menuitem"], [data-context-item]');
		const menuCount = await menuItems.count();
		for (let i = 0; i < menuCount; i++) {
			const text = await menuItems.nth(i).textContent();
			console.log(`Menu item ${i}: ${text}`);
		}
		await page.screenshot({ path: '.playwright-mcp/02-no-delete-option.png' });
		return;
	}
	
	await deleteOption.click();
	await page.waitForTimeout(500);
	
	// Check if confirmation dialog appeared
	const confirmDialog = page.locator('text=确定要删除, text=Delete this, text=Confirm');
	const dialogVisible = await confirmDialog.isVisible({ timeout: 3000 }).catch(() => false);
	console.log(`Confirm dialog visible: ${dialogVisible}`);
	
	if (dialogVisible) {
		// Click the Delete/Confirm button in the dialog
		const confirmBtn = page.locator('button:has-text("Delete"), button:has-text("删除")').last();
		await confirmBtn.click();
		await page.screenshot({ path: '.playwright-mcp/03-after-confirm.png' });
	}
	
	// Wait for toast
	await page.waitForTimeout(2000);
	
	// Check for toast with role="alert"
	const alerts = page.locator('[role="alert"]');
	const alertCount = await alerts.count();
	console.log(`Found ${alertCount} alert elements`);
	
	for (let i = 0; i < alertCount; i++) {
		const text = await alerts.nth(i).textContent();
		console.log(`Alert ${i}: ${text}`);
	}
	
	// Also check for any element containing "Deleted" or "删除成功"
	const deletedText = page.locator(':has-text("Deleted"), :has-text("删除成功"), :has-text("删除已加入队列")');
	const deletedCount = await deletedText.count();
	console.log(`Elements with delete feedback text: ${deletedCount}`);
	
	// Take final screenshot
	await page.screenshot({ path: '.playwright-mcp/04-final-state.png' });
	
	// The toast should exist (or have existed)
	const bodyText = await page.textContent('body');
	const hasFeedback = bodyText?.includes('Deleted') || bodyText?.includes('删除成功') || bodyText?.includes('Deletion queued') || bodyText?.includes('删除已加入队列');
	console.log(`Has feedback in body: ${hasFeedback}`);
});
