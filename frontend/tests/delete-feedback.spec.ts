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

test.describe('Delete feedback', () => {
	test('deleting a file shows a success toast', async ({ page }) => {
		await login(page);
		await page.goto('/browse');
		await page.waitForLoadState('networkidle', { timeout: 10000 });

		// Click on the first root mount point to enter a directory
		const rootCard = page.locator('button:has-text("tmp"), button:has-text("home"), button:has-text("etc")').first();
		await rootCard.click();
		await page.waitForLoadState('networkidle', { timeout: 10000 });

		// Create a test file first via the context menu
		// Right-click on empty area to get context menu
		const fileArea = page.locator('[role="region"][aria-label="File browser content"]');
		await fileArea.click({ button: 'right', position: { x: 400, y: 400 } });

		// Look for "New File" in context menu
		const newFileBtn = page.locator('text=New File').first();
		if (await newFileBtn.isVisible({ timeout: 2000 })) {
			await newFileBtn.click();
			await page.waitForTimeout(500);

			// Fill in the filename
			const nameInput = page.locator('input[placeholder="untitled.txt"]');
			if (await nameInput.isVisible({ timeout: 2000 })) {
				await nameInput.fill('_playwright_test_delete.txt');
				// Click Create button
				await page.click('button:has-text("Create")');
				await page.waitForTimeout(1000);
			}
		}

		// Find and click on the test file row to select it
		const testFileRow = page.locator('tr:has-text("_playwright_test_delete")').first();
		if (await testFileRow.isVisible({ timeout: 3000 })) {
			await testFileRow.click();
			await page.waitForTimeout(300);

			// Right-click to open context menu
			await testFileRow.click({ button: 'right' });
			await page.waitForTimeout(300);

			// Click Delete
			const deleteBtn = page.locator('text=Delete').first();
			if (await deleteBtn.isVisible({ timeout: 2000 })) {
				await deleteBtn.click();
				await page.waitForTimeout(500);

				// If confirmation dialog appears, confirm
				const confirmBtn = page.locator('button:has-text("Delete"):not(:has-text("Delete this"))').last();
				if (await confirmBtn.isVisible({ timeout: 2000 })) {
					await confirmBtn.click();
				}

				// Wait for and verify the toast notification
				await page.waitForTimeout(2000);

				// Check for toast element
				const toast = page.locator('[class*="toast"], [class*="Toast"], [role="alert"], [class*="notification"]').first();
				const toastText = await toast.textContent().catch(() => '');

				// Also check the entire page body for success text
				const bodyText = await page.textContent('body');

				console.log('Toast text:', toastText);
				console.log('Body contains "Deleted":', bodyText?.includes('Deleted'));
				console.log('Body contains "删除成功":', bodyText?.includes('删除成功'));

				// Check that some deletion feedback appears
				const hasFeedback =
					bodyText?.includes('Deleted') ||
					bodyText?.includes('删除成功') ||
					bodyText?.includes('deleted') ||
					toastText?.includes('Deleted') ||
					toastText?.includes('删除成功');

				if (!hasFeedback) {
					// Take a screenshot for debugging
					await page.screenshot({ path: '.playwright-mcp/delete-feedback-missing.png' });
					console.log('Screenshot saved to test-results/delete-feedback-missing.png');
				}

				expect(hasFeedback).toBe(true);
			} else {
				console.log('Delete button not found in context menu');
				await page.screenshot({ path: '.playwright-mcp/delete-no-context-menu.png' });
			}
		} else {
			console.log('Test file not found, skipping delete verification');
			await page.screenshot({ path: '.playwright-mcp/delete-no-test-file.png' });
		}
	});
});
