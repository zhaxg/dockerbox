import { defineConfig } from '@playwright/test';

export default defineConfig({
	testDir: './tests',
	// outputDir removed - use system temp directory instead of polluting project
	timeout: 30000,
	expect: { timeout: 10000 },
	use: {
		baseURL: 'http://localhost:8080',
		headless: true,
	},
	webServer: {
		command: 'bun run dev --host 0.0.0.0',
		url: 'http://localhost:8080',
		reuseExistingServer: true,
		timeout: 30000,
		cwd: '/root/boxbox/frontend',
	},
});
