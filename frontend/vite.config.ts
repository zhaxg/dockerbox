import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	css: {
		// Tailwind v4 plugins are auto-detected from package.json
	},
	server: {
		port: 8080,
		host: '0.0.0.0',
		allowedHosts: true,
		proxy: {
			'/api': {
				target: 'http://localhost:8081',
				changeOrigin: true
			},
			'/ws': {
				target: 'ws://localhost:8081',
				ws: true
			}
		}
	}
});
