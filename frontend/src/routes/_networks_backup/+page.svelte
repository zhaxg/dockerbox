<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { Card, Spinner, Button } from '$lib/components/ui';
	import { Network, Trash2, RefreshCw, AlertTriangle } from 'lucide-svelte';

	interface NetworkInfo {
		id: string;
		name: string;
		driver: string;
		scope: string;
		created: string;
		subnet: string;
		gateway: string;
		internal: boolean;
		containers: number;
	}

	let networks = $state<NetworkInfo[]>([]);
	let loading = $state(true);
	let pruning = $state(false);

	onMount(async () => {
		await loadNetworks();
	});

	async function loadNetworks() {
		loading = true;
		try {
			const data = await api.get<{ networks: NetworkInfo[] }>('/docker/networks');
			networks = data.networks || [];
		} catch (e) {
			console.error('Failed to load networks:', e);
		} finally {
			loading = false;
		}
	}

	let confirmDialog = $state<{ open: boolean; title: string; message: string; onConfirm: () => void }>({
		open: false, title: '', message: '', onConfirm: () => {}
	});

	function showConfirm(title: string, message: string, onConfirm: () => void) {
		confirmDialog = { open: true, title, message, onConfirm };
	}

	function closeConfirm() {
		confirmDialog.open = false;
	}

	async function removeNetwork(id: string) {
		showConfirm('删除网络', '确定要删除这个网络吗？', async () => {
			try {
				await api.delete(`/docker/networks/${id}`);
				await loadNetworks();
			} catch (e) {
				console.error('Failed to remove network:', e);
			}
		});
	}

	async function pruneNetworks() {
		showConfirm('清理未使用网络', '确定要清理所有未使用的网络吗？', async () => {
			pruning = true;
			try {
				await api.post('/docker/networks/prune');
				await loadNetworks();
			} catch (e) {
				console.error('Failed to prune networks:', e);
			} finally {
				pruning = false;
			}
		});
	}

	function getDriverColor(driver: string): string {
		switch (driver) {
			case 'bridge':
				return 'bg-blue-500/10 text-blue-500';
			case 'host':
				return 'bg-green-500/10 text-green-500';
			case 'overlay':
				return 'bg-purple-500/10 text-purple-500';
			default:
				return 'bg-gray-500/10 text-gray-500';
		}
	}
</script>

<div class="p-6 bg-surface-primary">
	<div class="mb-6 flex items-center justify-between">
		<h1 class="text-2xl font-semibold text-text-primary">网络管理</h1>
		<div class="flex gap-2">
			<Button variant="secondary" onclick={loadNetworks}>
				<RefreshCw size={16} class="mr-2" />
				刷新
			</Button>
			<Button variant="danger" onclick={pruneNetworks} disabled={pruning}>
				<AlertTriangle size={16} class="mr-2" />
				{pruning ? '清理中...' : '清理未使用网络'}
			</Button>
		</div>
	</div>

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<Spinner size="lg" />
		</div>
	{:else if networks.length === 0}
		<Card>
			<div class="py-8 text-center">
				<Network size={48} class="mx-auto mb-4 text-text-muted" />
				<p class="text-text-secondary">暂无网络</p>
			</div>
		</Card>
	{:else}
		<div class="grid gap-4">
			{#each networks as network (network.id)}
				<Card>
					<div class="flex items-center justify-between">
						<div class="flex items-center gap-4">
							<div class="rounded-lg {getDriverColor(network.driver)} p-2">
								<Network size={24} />
							</div>
							<div>
								<div class="font-medium text-text-primary">{network.name}</div>
								<div class="text-sm text-text-secondary">
									{network.driver} | {network.scope}
									{#if network.subnet}
										| {network.subnet}
									{/if}
								</div>
							</div>
						</div>

						<div class="flex items-center gap-4">
							<div class="text-right text-sm">
								<div class="text-text-secondary">
									{new Date(network.created).toLocaleDateString()}
								</div>
								{#if network.internal}
									<div class="text-xs text-yellow-500">内部网络</div>
								{/if}
							</div>

							<Button
								variant="danger"
								size="sm"
								onclick={() => removeNetwork(network.id)}
								title="删除网络"
							>
								<Trash2 size={14} />
							</Button>
						</div>
					</div>
				</Card>
			{/each}
		</div>
	{/if}

{#if confirmDialog.open}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
		<div class="w-96 rounded-lg bg-surface-primary p-6 shadow-xl border border-border-secondary">
			<h3 class="mb-2 text-lg font-semibold text-text-primary">{confirmDialog.title}</h3>
			<p class="mb-6 text-sm text-text-secondary">{confirmDialog.message}</p>
			<div class="flex justify-end gap-2">
				<Button variant="secondary" onclick={closeConfirm}>取消</Button>
				<Button variant="danger" onclick={() => { confirmDialog.onConfirm(); closeConfirm(); }}>确定</Button>
			</div>
		</div>
	</div>
{/if}

</div>
