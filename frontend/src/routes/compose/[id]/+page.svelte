<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { Card, Spinner, Button } from '$lib/components/ui';
	import { ArrowLeft, Play, StopCircle, Hammer, RefreshCw, Save, RotateCcw, Download } from 'lucide-svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import CodePreview from '$lib/components/preview/CodePreview.svelte';

	const projectId = $derived(page.params.id);

	let composeFile = $state('');
	let envFile = $state('');
	let loading = $state(true);
	let saving = $state(false);
	let activeTab = $state<'compose' | 'env'>('compose');

	onMount(async () => {
		await loadFiles();
	});

	async function loadFiles() {
		loading = true;
		try {
			const [composeData, envData] = await Promise.all([
				api.get<{ content: string }>(`/docker/compose/${projectId}/file`).catch(() => ({ content: '' })),
				api.get<{ content: string }>(`/docker/compose/${projectId}/env`).catch(() => ({ content: '' }))
			]);
			composeFile = composeData.content || '';
			envFile = envData.content || '';
		} catch (e) {
			console.error('Failed to load files:', e);
		} finally {
			loading = false;
		}
	}

	async function saveComposeFile() {
		saving = true;
		try {
			await api.put(`/docker/compose/${projectId}/file`, { content: composeFile });
		} catch (e) {
			console.error('Failed to save compose file:', e);
		} finally {
			saving = false;
		}
	}

	async function saveEnvFile() {
		saving = true;
		try {
			await api.put(`/docker/compose/${projectId}/env`, { content: envFile });
		} catch (e) {
			console.error('Failed to save env file:', e);
		} finally {
			saving = false;
		}
	}

	async function composeUp() {
		await api.post(`/docker/compose/${projectId}/up`);
	}

	async function composeDown() {
		await api.post(`/docker/compose/${projectId}/down`);
	}

	async function composeBuild() {
		await api.post(`/docker/compose/${projectId}/build`);
	}

	async function composeRestart() {
		await api.post(`/docker/compose/${projectId}/restart`);
	}

	async function composeRedeploy() {
		if (!confirm('确定要重新部署这个项目吗？')) return;
		await api.post(`/docker/compose/${projectId}/redeploy`);
	}
</script>

<div class="flex h-full flex-col p-6">
	<!-- Header -->
	<div class="mb-6 flex items-center justify-between">
		<div class="flex items-center gap-4">
			<Button variant="secondary" onclick={() => goto(resolve('/compose'))}>
				<ArrowLeft size={16} class="mr-2" />
				返回
			</Button>
			<h1 class="text-2xl font-semibold text-text-primary">{projectId}</h1>
		</div>
		<div class="flex gap-2">
			<Button variant="primary" size="sm" onclick={composeUp} title="启动"><Play size={14} class="mr-1" />启动</Button>
			<Button variant="secondary" size="sm" onclick={composeDown} title="停止"><StopCircle size={14} class="mr-1" />停止</Button>
			<Button variant="secondary" size="sm" onclick={composeRestart} title="重启"><RotateCcw size={14} class="mr-1" />重启</Button>
			<Button variant="secondary" size="sm" onclick={composeBuild} title="构建"><Hammer size={14} class="mr-1" />构建</Button>
			<Button variant="secondary" size="sm" onclick={composeRedeploy} title="重新部署"><RefreshCw size={14} class="mr-1" />重新部署</Button>
		</div>
	</div>

	{#if loading}
		<Spinner size="lg" />
	{:else}
		<!-- Tabs -->
		<div class="mb-4 flex gap-2 border-b border-border-secondary">
			<button
				class="border-b-2 px-4 py-2 text-sm font-medium transition-colors {activeTab === 'compose' ? 'border-accent text-accent' : 'border-transparent text-text-secondary hover:text-text-primary'}"
				onclick={() => (activeTab = 'compose')}
			>
				docker-compose.yml
			</button>
			<button
				class="border-b-2 px-4 py-2 text-sm font-medium transition-colors {activeTab === 'env' ? 'border-accent text-accent' : 'border-transparent text-text-secondary hover:text-text-primary'}"
				onclick={() => (activeTab = 'env')}
			>
				.env
			</button>
		</div>

		<!-- Editor -->
		<div class="flex-1">
			{#if activeTab === 'compose'}
				<div class="h-[500px]">
					<CodePreview
						url={`/api/v1/docker/compose/${projectId}/file`}
						filename="docker-compose.yml"
						path={`/docker/compose/${projectId}/file`}
						onSaved={() => loadFiles()}
					/>
				</div>
			{:else}
				<div class="h-[500px]">
					<CodePreview
						url={`/api/v1/docker/compose/${projectId}/env`}
						filename=".env"
						path={`/docker/compose/${projectId}/env`}
						onSaved={() => loadFiles()}
					/>
				</div>
			{/if}
		</div>
	{/if}
</div>
