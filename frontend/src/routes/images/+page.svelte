<script lang="ts">
	import { onMount } from 'svelte';
	import { Card, Spinner, Button } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import { Image, RefreshCw, Trash2, Download, AlertTriangle } from 'lucide-svelte';

	interface DockerImage {
		Id: string;
		ParentId: string;
		Created: number;
		Size: number;
		SharedSize: number;
		VirtualSize: number;
		Containers: number;
		RepoTags: string[];
		RepoDigests: string[];
	}

	let images = $state<DockerImage[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let pulling = $state(false);
	let pruning = $state(false);
	let pullImageName = $state('');

	onMount(async () => {
		await loadImages();
	});

	async function loadImages() {
		loading = true;
		error = null;
		try {
			const data = await api.get<{ images: DockerImage[] }>('/docker/images');
			images = data.images || [];
		} catch (e) {
			error = e instanceof Error ? e.message : 'Unknown error';
		} finally {
			loading = false;
		}
	}

	async function deleteImage(id: string) {
		if (!confirm('确定要删除此镜像吗？')) return;
		try {
			await api.delete(`/docker/images/${id}`);
			await loadImages();
		} catch (e) {
			console.error('Failed to delete image:', e);
		}
	}

	async function pullImage() {
		if (!pullImageName.trim()) return;
		pulling = true;
		try {
			await api.post('/docker/images/pull', { image: pullImageName.trim() });
			await loadImages();
			pullImageName = '';
		} catch (e) {
			console.error('Failed to pull image:', e);
			error = e instanceof Error ? e.message : 'Failed to pull image';
		} finally {
			pulling = false;
		}
	}

	async function pruneImages() {
		if (!confirm('确定要清理所有未使用的镜像吗？')) return;
		pruning = true;
		try {
			await api.post('/docker/images/prune');
			await loadImages();
		} catch (e) {
			console.error('Failed to prune images:', e);
		} finally {
			pruning = false;
		}
	}

	function formatSize(bytes: number): string {
		if (bytes === 0) return '0 B';
		const k = 1024;
		const sizes = ['B', 'KB', 'MB', 'GB'];
		const i = Math.floor(Math.log(bytes) / Math.log(k));
		return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
	}

	function formatDate(timestamp: number): string {
		return new Date(timestamp * 1000).toLocaleDateString('zh-CN');
	}

	function getTag(image: DockerImage): string {
		if (image.RepoTags && image.RepoTags.length > 0 && image.RepoTags[0] !== '<none>:<none>') {
			return image.RepoTags[0];
		}
		return image.Id.substring(0, 12);
	}
</script>

<div class="p-6">
	<div class="mb-6 flex items-center justify-between">
		<h1 class="text-2xl font-semibold text-text-primary">镜像管理</h1>
		<div class="flex gap-2">
			<Button variant="secondary" onclick={loadImages}>
				<RefreshCw size={16} class="mr-2" />
				刷新
			</Button>
			<Button variant="danger" onclick={pruneImages} disabled={pruning}>
				<AlertTriangle size={16} class="mr-2" />
				{pruning ? '清理中...' : '清理未使用镜像'}
			</Button>
		</div>
	</div>


	{#if loading}
		<div class="flex items-center justify-center py-12">
			<Spinner size="lg" />
		</div>
	{:else if error}
		<Card>
			<div class="text-center text-red-500">
				<p>{error}</p>
				<Button variant="secondary" onclick={loadImages} class="mt-4">重试</Button>
			</div>
		</Card>
	{:else if images.length === 0}
		<Card>
			<div class="py-8 text-center">
				<Image size={48} class="mx-auto mb-4 text-text-muted" />
				<p class="text-text-secondary">暂无镜像</p>
			</div>
		</Card>
	{:else}
		<div class="grid gap-4">
			{#each images as image (image.Id)}
				<Card>
					<div class="flex items-center justify-between">
						<div class="flex items-center gap-4">
							<div class="rounded-lg bg-cyan-500/10 p-2">
								<Image size={20} class="text-cyan-500" />
							</div>
							<div>
								<div class="font-medium text-text-primary">{getTag(image)}</div>
								<div class="text-sm text-text-secondary">
									ID: {image.Id.substring(0, 12)}
								</div>
							</div>
						</div>

						<div class="flex items-center gap-6">
							<!-- Size -->
							<div class="text-right text-sm">
								<div class="text-text-secondary">{formatSize(image.Size)}</div>
								<div class="text-text-muted">创建于 {formatDate(image.Created)}</div>
							</div>

							<!-- Containers -->
							<div class="text-right text-sm">
								<div class="text-text-secondary">{image.Containers < 0 ? '?' : image.Containers} 个容器</div>
							</div>

							<!-- Actions -->
							<div class="flex gap-2">
								<Button variant="danger" size="sm" onclick={() => deleteImage(image.Id)}>
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
