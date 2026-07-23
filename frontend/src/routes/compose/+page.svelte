<script lang="ts">
	import { onMount } from 'svelte';
	import { Card, Spinner, Button } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import {
		Play,
		StopCircle,
		RefreshCw,
		Trash2,
		Package,
		Hammer,
		FileText,
		ArrowUpCircle,
		ArrowDownCircle,
		Eye,
		RotateCcw,
		Download
	} from 'lucide-svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';

	interface ComposeProject {
		id: string;
		name: string;
		path: string;
		status: string;
		services: number;
		running: number;
	}

	let projects = $state<ComposeProject[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	onMount(async () => {
		await loadProjects();
	});

	async function loadProjects() {
		loading = true;
		error = null;
		try {
			const data = await api.get<{ projects: ComposeProject[] }>('/docker/compose');
			projects = data.projects || [];
		} catch (e) {
			error = e instanceof Error ? e.message : 'Unknown error';
		} finally {
			loading = false;
		}
	}

	async function composeUp(id: string) {
		try {
			await api.post(`/docker/compose/${id}/up`);
			await loadProjects();
		} catch (e) {
			console.error('Failed to start compose:', e);
		}
	}

	async function composeDown(id: string) {
		try {
			await api.post(`/docker/compose/${id}/down`);
			await loadProjects();
		} catch (e) {
			console.error('Failed to stop compose:', e);
		}
	}

	async function composeBuild(id: string) {
		try {
			await api.post(`/docker/compose/${id}/build`);
			await loadProjects();
		} catch (e) {
			console.error('Failed to build compose:', e);
		}
	}

	async function composeRestart(id: string) {
		try {
			await api.post(`/docker/compose/${id}/restart`);
			await loadProjects();
		} catch (e) {
			console.error('Failed to restart compose:', e);
		}
	}

	async function composeRedeploy(id: string) {
		if (!confirm('确定要重新部署这个项目吗？这将停止并重新启动所有服务。')) return;
		try {
			await api.post(`/docker/compose/${id}/redeploy`);
			await loadProjects();
		} catch (e) {
			console.error('Failed to redeploy compose:', e);
		}
	}

	function getStatusColor(status: string): string {
		switch (status) {
			case 'running':
				return 'bg-green-500';
			case 'stopped':
				return 'bg-red-500';
			case 'partial':
				return 'bg-yellow-500';
			default:
				return 'bg-gray-500';
		}
	}

	function getStatusText(status: string): string {
		switch (status) {
			case 'running':
				return '运行中';
			case 'stopped':
				return '已停止';
			case 'partial':
				return '部分运行';
			default:
				return status;
		}
	}
</script>

<div class="p-6">
	<div class="mb-6 flex items-center justify-between">
		<h1 class="text-2xl font-semibold text-text-primary">Compose 管理</h1>
		<Button variant="secondary" onclick={loadProjects}>
			<RefreshCw size={16} class="mr-2" />
			刷新
		</Button>
	</div>

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<Spinner size="lg" />
		</div>
	{:else if error}
		<Card>
			<div class="text-center text-red-500">
				<p>{error}</p>
				<Button variant="secondary" onclick={loadProjects} class="mt-4">重试</Button>
			</div>
		</Card>
	{:else if projects.length === 0}
		<Card>
			<div class="py-8 text-center">
				<Package size={48} class="mx-auto mb-4 text-text-muted" />
				<p class="text-text-secondary">暂无 Compose 项目</p>
			</div>
		</Card>
	{:else}
		<div class="grid gap-4">
			{#each projects as project (project.id)}
				<Card>
					<div class="flex items-center justify-between">
						<div class="flex items-center gap-4">
							<div class="h-3 w-3 rounded-full {getStatusColor(project.status)}"></div>
							<div>
								<div class="font-medium text-text-primary">{project.name}</div>
								<div class="text-sm text-text-secondary">{project.path}</div>
							</div>
						</div>

						<div class="flex items-center gap-6">
							<!-- Status -->
							<div class="text-right text-sm">
								<div class="text-text-secondary">{getStatusText(project.status)}</div>
								<div class="text-text-secondary">
									服务: {project.running}/{project.services}
								</div>
							</div>

							<!-- Actions -->
							<div class="flex gap-2">
								{#if project.status === 'running'}
									<Button variant="secondary" size="sm" onclick={() => composeDown(project.id)} title="停止">
										<ArrowDownCircle size={14} />
									</Button>
									<Button variant="secondary" size="sm" onclick={() => composeRestart(project.id)} title="重启">
										<RotateCcw size={14} />
									</Button>
								{:else}
									<Button variant="primary" size="sm" onclick={() => composeUp(project.id)} title="启动">
										<ArrowUpCircle size={14} />
									</Button>
								{/if}
								<Button variant="secondary" size="sm" onclick={() => composeBuild(project.id)} title="构建">
									<Hammer size={14} />
								</Button>
								<Button variant="secondary" size="sm" onclick={() => composeRedeploy(project.id)} title="重新部署">
									<RefreshCw size={14} />
								</Button>
								<Button variant="secondary" size="sm" onclick={() => goto(resolve(`/compose/${project.id}`))} title="查看">
									<Eye size={14} />
								</Button>
							</div>
						</div>
					</div>
				</Card>
			{/each}
		</div>
	{/if}
</div>
