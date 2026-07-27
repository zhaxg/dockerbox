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

test('selection colors in light mode', async ({ page }) => {
	await login(page);

	// Switch to light mode first via settings store
	await page.evaluate(() => {
		const settings = JSON.parse(localStorage.getItem('dockerbox_settings') || '{}');
		settings.theme = 'light';
		localStorage.setItem('dockerbox_settings', JSON.stringify(settings));
	});

	// Reload to apply theme
	await page.goto('/browse');
	await page.waitForLoadState('networkidle', { timeout: 10000 });

	// Click a root mount point
	const rootCard = page.locator('button:has-text("tmp"), button:has-text("home"), button:has-text("etc")').first();
	if (await rootCard.isVisible({ timeout: 3000 })) {
		await rootCard.click();
		await page.waitForLoadState('networkidle', { timeout: 10000 });
		await page.waitForTimeout(1000);
	}

	// Screenshot before selection
	await page.screenshot({ path: '.playwright-mcp/selection-before.png' });

	// Click first file row to select it
	const firstRow = page.locator('tr[data-file-row="true"]').first();
	if (await firstRow.isVisible({ timeout: 3000 })) {
		await firstRow.click();
		await page.waitForTimeout(500);

		// Screenshot after selection
		await page.screenshot({ path: '.playwright-mcp/selection-after.png' });

		// Get computed styles of the selected row
		const styles = await firstRow.evaluate((el) => {
			const computed = window.getComputedStyle(el);
			const firstTd = el.querySelector('td');
			const tdComputed = firstTd ? window.getComputedStyle(firstTd) : null;
			const firstSpan = firstTd?.querySelector('span');
			const spanComputed = firstSpan ? window.getComputedStyle(firstSpan) : null;
			return {
				trBg: computed.backgroundColor,
				trColor: computed.color,
				trAriaSelected: el.getAttribute('aria-selected'),
				tdColor: tdComputed?.color,
				spanColor: spanComputed?.color,
				// Also get the actual CSS variables
				theme: document.documentElement.getAttribute('data-theme'),
				selectionVar: getComputedStyle(document.documentElement).getPropertyValue('--color-selection'),
				textPrimaryVar: getComputedStyle(document.documentElement).getPropertyValue('--color-text-primary'),
				textSecondaryVar: getComputedStyle(document.documentElement).getPropertyValue('--color-text-secondary'),
			};
		});
		console.log('Selected row styles:', JSON.stringify(styles, null, 2));

		// Get styles of a non-selected row for comparison
		const secondRow = page.locator('tr[data-file-row="true"]').nth(1);
		if (await secondRow.isVisible({ timeout: 2000 })) {
			const unselectedStyles = await secondRow.evaluate((el) => {
				const firstTd = el.querySelector('td');
				const tdComputed = firstTd ? window.getComputedStyle(firstTd) : null;
				const firstSpan = firstTd?.querySelector('span');
				const spanComputed = firstSpan ? window.getComputedStyle(firstSpan) : null;
				return {
					tdColor: tdComputed?.color,
					spanColor: spanComputed?.color,
				};
			});
			console.log('Unselected row styles:', JSON.stringify(unselectedStyles, null, 2));
		}
	}
});
