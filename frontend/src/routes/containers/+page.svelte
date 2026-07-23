<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { Card, Spinner, Button } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import {
		Play,
		StopCircle,
		RefreshCw,
		Trash2,
		FileText,
		ExternalLink,
		Container,
		Eye,
		Skull
	} from 'lucide-svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';

	interface PortBinding {
		hostIp: string;
		hostPort: string;
		containerPort: string;
		protocol: string;
	}

	interface ContainerInfo {
		id: string;
		name: string;
		image: string;
		status: string;
		state: string;
		created: string;
		ports: PortBinding[];
		cpu: number;
		memory: {
			usage: number;
			limit: number;
			percent: number;
		};
	}

	let containers = $state<ContainerInfo[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let eventSource: EventSource | null = null;

	onMount(async () => {
		await loadContainers();
		connectSSE();
	});

	onDestroy(() => {
		if (eventSource) {
			eventSource.close();
		}
	});

	async function loadContainers() {
		loading = true;
		error = null;
		try {
			const data = await api.get<{ containers: ContainerInfo[] }>('/docker/containers');
			containers = data.containers || [];
		} catch (e) {
			error = e instanceof Error ? e.message : 'Unknown error';
		} finally {
			loading = false;
		}
	}

	function connectSSE() {
		const token = localStorage.getItem('token');
		if (!token) return;

		eventSource = new EventSource(`/api/v1/sse/stats?token=${token}`);

		eventSource.addEventListener('stats', (event) => {
			try {
				const data = JSON.parse(event.data);
				// Update container stats in real-time
				// This is a simplified version - in production, you'd match by container ID
				console.log('Container stats update:', data);
			} catch (e) {
				console.error('Failed to parse stats:', e);
			}
		});

		eventSource.addEventListener('error', (event) => {
			console.error('SSE error:', event);
		});
	}

	async function startContainer(id: string) {
		try {
			await api.post(`/docker/containers/${id}/start`);
			await loadContainers();
		} catch (e) {
			console.error('Failed to start container:', e);
		}
	}

	async function stopContainer(id: string) {
		try {
			await api.post(`/docker/containers/${id}/stop`);
			await loadContainers();
		} catch (e) {
			console.error('Failed to stop container:', e);
		}
	}

	async function restartContainer(id: string) {
		try {
			await api.post(`/docker/containers/${id}/restart`);
			await loadContainers();
		} catch (e) {
			console.error('Failed to restart container:', e);
		}
	}

	async function deleteContainer(id: string) {
		if (!confirm('确定要删除这个容器吗？')) return;
		try {
			await api.delete(`/docker/containers/${id}`);
			await loadContainers();
		} catch (e) {
			console.error('Failed to delete container:', e);
		}
	}

	async function killContainer(id: string, signal: string = 'SIGKILL') {
		if (!confirm(`确定要终止这个容器吗？(信号: ${signal})`)) return;
		try {
			await api.post(`/docker/containers/${id}/kill?signal=${signal}`);
			await loadContainers();
		} catch (e) {
			console.error('Failed to kill container:', e);
		}
	}

	function getStateColor(state: string): string {
		switch (state) {
			case 'running':
				return 'bg-green-500';
			case 'exited':
				return 'bg-red-500';
			case 'paused':
				return 'bg-yellow-500';
			default:
				return 'bg-gray-500';
		}
	}

	function formatBytes(bytes: number): string {
		if (bytes === 0) return '0 B';
		const k = 1024;
		const sizes = ['B', 'KB', 'MB', 'GB'];
		const i = Math.floor(Math.log(bytes) / Math.log(k));
		return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
	}
</script>

<div class="p-6">
	<div class="mb-6 flex items-center justify-between">
		<h1 class="text-2xl font-semibold text-text-primary">容器管理</h1>
		<Button variant="secondary" onclick={loadContainers}>
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
				<Button variant="secondary" onclick={loadContainers} class="mt-4">重试</Button>
			</div>
		</Card>
	{:else if containers.length === 0}
		<Card>
			<div class="py-8 text-center">
				<Container size={48} class="mx-auto mb-4 text-text-muted" />
				<p class="text-text-secondary">暂无容器</p>
			</div>
		</Card>
	{:else}
		<div class="grid gap-4">
			{#each containers as container (container.id)}
				<Card>
					<div class="flex items-center justify-between">
						<div class="flex items-center gap-4">
							<div class="h-3 w-3 rounded-full {getStateColor(container.state)}"></div>
							<div>
								<div class="font-medium text-text-primary">{container.name}</div>
								<div class="text-sm text-text-secondary">{container.image}</div>
							</div>
						</div>

						<div class="flex items-center gap-6">
							<!-- Resource Usage -->
							<div class="text-right text-sm">
								<div class="text-text-secondary">
									CPU: {container.cpu?.toFixed(1) || 0}%
								</div>
								<div class="text-text-secondary">
									MEM: {formatBytes(container.memory?.usage || 0)}
								</div>
							</div>

							<!-- Ports -->
							{#if container.ports?.length > 0}
								<div class="flex gap-2">
									{#each container.ports.slice(0, 3) as port}
										{#if port.hostPort}
											<a
												href="http://localhost:{port.hostPort}"
												target="_blank"
												rel="noopener noreferrer"
												class="flex items-center gap-1 rounded bg-blue-500/10 px-2 py-1 text-xs text-blue-500 hover:bg-blue-500/20"
											>
												{port.hostPort}
												<ExternalLink size={12} />
											</a>
										{/if}
									{/each}
								</div>
							{/if}

							<!-- Actions -->
							<div class="flex gap-2">
							{#if container.state === 'running'}
							<Button variant="secondary" size="sm" onclick={() => stopContainer(container.id)}>
							<StopCircle size={14} />
							</Button>
							<Button variant="secondary" size="sm" onclick={() => restartContainer(container.id)}>
							<RefreshCw size={14} />
							</Button>
							 <Button variant="danger" size="sm" onclick={() => killContainer(container.id)}>
							 <Skull size={14} />
							</Button>
							{:else}
							 <Button variant="primary" size="sm" onclick={() => startContainer(container.id)}>
							  <Play size={14} />
							</Button>
							{/if}
							<Button variant="secondary" size="sm" onclick={() => goto(resolve(`/containers/${container.id}`))}>
							<Eye size={14} />
							</Button>
							 <Button variant="danger" size="sm" onclick={() => deleteContainer(container.id)}>
								<Trash2 size={14} />
							</Button>
						</div>
						</div>
					</div>
				</Card>
			{/each}
		</div>
	{/if}
</div>
