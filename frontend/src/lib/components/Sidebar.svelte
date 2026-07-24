<script lang="ts">
	/**
	 * Sidebar component - navigation panel with Docker management
	 */
	import {
		LayoutDashboard,
		Container,
		Package,
		FolderOpen,
		Star,
		Settings,
		ChevronDown,
		Server,
		Network
	} from 'lucide-svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { settingsStore } from '$lib/stores/settings';
	import { listRoots, type MountPoint } from '$lib/api/files';
	import { hostsApi, type DockerHostsConfig } from '$lib/api/hosts';
	import { onMount } from 'svelte';

	let mountPoints = $state<MountPoint[]>([]);
	let hostsConfig = $state<DockerHostsConfig>({ default: '', hosts: {} });
	let hostDropdownOpen = $state(false);

	// Navigation items
	const navItems = [
		{ name: '概览', path: '/overview', icon: LayoutDashboard },
		{ name: '主机', path: '/hosts', icon: Server },
		{ name: '容器', path: '/containers', icon: Container },
		{ name: 'Compose', path: '/compose', icon: Package },
		{
			name: '文件',
			path: '/browse',
			icon: FolderOpen,
			children: [
				{ name: '收藏目录', path: '/browse/favorites', icon: Star, isFavorites: true },
				{ name: '根目录', path: '/browse', icon: Server }
			]
		},
		{ name: '设置', path: '/settings', icon: Settings }
	];

	let filesExpanded = $state(true);

	onMount(async () => {
		try {
			const [mpData, hostData] = await Promise.all([
				listRoots().catch(() => ({ roots: [] })),
				hostsApi.list().catch(() => ({ default: '', hosts: {} }))
			]);
			mountPoints = mpData.roots || [];
			hostsConfig = hostData;
			if (!hostsConfig.hosts) hostsConfig.hosts = {};
		} catch (e) {
			console.error('Failed to load data:', e);
		}
	});

	async function switchHost(id: string) {
		hostsConfig.default = id;
		hostDropdownOpen = false;
		localStorage.setItem('currentHostId', id);
		window.dispatchEvent(new Event('host-changed'));
		try {
			const host = hostsConfig.hosts[id];
			if (host) await hostsApi.update(id, { ...host, isDefault: true });
		} catch (e) { console.error(e); }
	}

	function isActive(path: string): boolean {
		return page.url.pathname.startsWith(path);
	}

	function handleNavigate(path: string) {
		goto(resolve(path));
	}
</script>

<aside
	class="flex w-[220px] min-w-[220px] flex-col overflow-x-hidden overflow-y-auto border-r border-border-secondary bg-surface-primary"
>
	<!-- Logo -->
	<div class="border-b border-border-secondary px-4 py-3">
		<h1 class="text-lg font-semibold text-text-primary">BoxBox</h1>
	</div>

	<!-- Host Switcher -->
	{#if Object.keys(hostsConfig.hosts || {}).length > 0}
	<div class="border-b border-border-secondary px-3 py-2">
		<button type="button" class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-[12px] text-text-secondary hover:bg-surface-secondary transition-colors"
			onclick={() => hostDropdownOpen = !hostDropdownOpen}>
			<Server size={14} class="shrink-0 text-green-500" />
			<span class="flex-1 text-left truncate text-text-primary font-medium">{hostsConfig.hosts[hostsConfig.default]?.name || '选择主机'}</span>
			<ChevronDown size={12} class="shrink-0 transition-transform {hostDropdownOpen ? '' : '-rotate-90'}" />
		</button>
		{#if hostDropdownOpen}
			<div class="mt-1 space-y-0.5">
				{#each Object.entries(hostsConfig.hosts || {}) as [id, host]}
					<button type="button" class="flex w-full items-center gap-2 rounded px-2 py-1 text-[12px] hover:bg-surface-secondary transition-colors {hostsConfig.default === id ? 'text-green-400' : 'text-text-secondary'}"
						onclick={() => switchHost(id)}>
						<span class="h-1.5 w-1.5 rounded-full {hostsConfig.default === id ? 'bg-green-500' : 'bg-gray-500'}"></span>
						<span class="flex-1 text-left truncate">{host.name}</span>
					</button>
				{/each}
			</div>
		{/if}
	</div>
	{/if}

	<!-- Main Navigation -->
	<nav class="flex-1 overflow-y-auto py-2">
		{#each navItems as item}
			{#if item.children}
				<!-- Navigation item with children (Files) -->
				<button
					type="button"
					class="nav-item {isActive(item.path) ? 'active' : ''}"
					onclick={() => {
						filesExpanded = !filesExpanded;
						handleNavigate(item.path);
					}}
				>
					<item.icon size={18} class="shrink-0 opacity-80" />
					<span class="flex-1 text-left">{item.name}</span>
					<ChevronDown
						size={14}
						class="shrink-0 transition-transform duration-150 {filesExpanded ? '' : '-rotate-90'}"
					/>
				</button>

				{#if filesExpanded}
				<div class="ml-4">
				<!-- Favorites section -->
				{#if item.children[0].isFavorites}
				<div class="nav-section-title">收藏目录</div>
				{#each $settingsStore.favoriteFolders as fav}
				<button
				type="button"
				class="nav-item-sub {isActive(`/browse/${fav.path}`) ? 'active' : ''}"
				onclick={() => handleNavigate(`/browse/${fav.path}`)}
				>
				<Star size={14} class="shrink-0 opacity-80" />
				<span class="flex-1 overflow-hidden text-ellipsis whitespace-nowrap">{fav.name}</span>
				</button>
				{/each}

						<!-- Mount points -->
				 <div class="nav-section-title">挂载目录</div>
				 {#each mountPoints as mp}
				 <button
				 type="button"
				 class="nav-item-sub {isActive(`/browse/${mp.name}`) ? 'active' : ''}"
				 onclick={() => handleNavigate(`/browse/${mp.name}`)}
				 >
				 <Server size={14} class="shrink-0 opacity-80" />
				 <span class="flex-1 overflow-hidden text-ellipsis whitespace-nowrap">{mp.name}</span>
				 </button>
				 {/each}
				 {/if}

					<!-- Other directories -->
					{#each item.children.slice(1) as child}
						<button
							type="button"
							class="nav-item-sub {isActive(child.path) ? 'active' : ''}"
							onclick={() => handleNavigate(child.path)}
						>
							<child.icon size={14} class="shrink-0 opacity-80" />
							<span class="flex-1 text-left">{child.name}</span>
						</button>
					{/each}
				</div>
			{/if}
			{:else}
				<!-- Simple navigation item -->
				<button
					type="button"
					class="nav-item {isActive(item.path) ? 'active' : ''}"
					onclick={() => handleNavigate(item.path)}
				>
					<item.icon size={18} class="shrink-0 opacity-80" />
					<span class="flex-1 text-left">{item.name}</span>
				</button>
			{/if}
		{/each}
	</nav>
</aside>

<style>
	.nav-item {
		display: flex;
		width: 100%;
		cursor: pointer;
		align-items: center;
		gap: 10px;
		border: none;
		background: transparent;
		padding: 8px 16px;
		text-align: left;
		font-size: 13px;
		color: var(--color-text-primary);
		transition: background-color 100ms;
	}
	.nav-item:hover {
		background: var(--color-surface-secondary);
	}
	.nav-item.active {
		background: var(--color-selection);
		color: white;
	}
	.nav-item.active:hover {
		background: var(--color-selection-hover);
	}
	.nav-item-sub {
		display: flex;
		width: 100%;
		cursor: pointer;
		align-items: center;
		gap: 8px;
		border: none;
		background: transparent;
		padding: 6px 16px;
		text-align: left;
		font-size: 12px;
		color: var(--color-text-secondary);
		transition: background-color 100ms;
	}
	.nav-item-sub:hover {
		background: var(--color-surface-secondary);
		color: var(--color-text-primary);
	}
	.nav-item-sub.active {
		color: var(--color-accent);
	}
	.nav-section-title {
		padding: 4px 16px;
		font-size: 11px;
		font-weight: 500;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-text-muted);
	}
</style>
