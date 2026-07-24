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
	import { onMount } from 'svelte';

	let mountPoints = $state<MountPoint[]>([]);

	// Navigation items
	const navItems = [
		{ name: '概览', path: '/overview', icon: LayoutDashboard },
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
			const data = await listRoots();
			mountPoints = data.roots || [];
		} catch (e) {
			console.error('Failed to load mount points:', e);
		}
	});

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
