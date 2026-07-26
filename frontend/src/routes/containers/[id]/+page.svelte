<script lang="ts">
	import { t, getLocale } from '$lib/i18n/index.svelte';
	import { page } from '$app/state';
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api/client';
	import { dockerApi } from '$lib/api/docker';
	import { Card, Spinner, Button } from '$lib/components/ui';
	import { ArrowLeft, Play, StopCircle, RefreshCw, Trash2, Terminal, Skull, Info } from 'lucide-svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	const tContainersBack = $derived(t("containers.back"));
	const tContainersImage = $derived(t("containers.image"));
	const tContainersLine = $derived(t("containers.line"));
	const tContainersLogsstream = $derived(t("containers.logsStream"));
	const tContainersStopstream = $derived(t("containers.stopStream"));
	const tContainersNologs = $derived(t("containers.noLogs"));
	const tContainersRestart = $derived(t("containers.restart"));
	const tContainersStart = $derived(t("containers.start"));
	const tContainersTerminal = $derived(t("containers.terminal"));
	const tCommonStatus = $derived(t("common.status"));
	const tCommonPort = $derived(t("common.port"));
	const tCommonClose = $derived(t("common.close"));
	const tComposeLogs = $derived(t("compose.logs"));
	const tComposeStop = $derived(t("compose.stop"));

	const containerId = $derived(page.params.id);

	interface ContainerInfo {
		id: string;
		name: string;
		image: string;
		status: string;
		state: string;
		ports: { hostPort: string; containerPort: string; protocol: string }[];
	}

	let container = $state<ContainerInfo | null>(null);
	let logs = $state<string[]>([]);
	let loading = $state(true);
	let logsLoading = $state(false);
	let tail = $state(100);
	let eventSource: EventSource | null = null;
	let streaming = $state(false);
	let inspectData = $state<any>(null);
	let showInspect = $state(false);

	onMount(async () => {
		await loadContainer();
		await loadLogs();
	});

	onDestroy(() => {
		if (eventSource) {
			eventSource.close();
		}
	});

	async function loadContainer() {
		try {
			container = await dockerApi.get<ContainerInfo>(`/docker/containers/${containerId}`);
		} catch (e) {
			console.error('Failed to load container:', e);
		} finally {
			loading = false;
		}
	}

	async function loadLogs() {
		logsLoading = true;
		try {
			const data = await dockerApi.get<{ lines: string[] }>(`/docker/containers/${containerId}/logs?tail=${tail}`);
			logs = data.lines || [];
		} catch (e) {
			console.error('Failed to load logs:', e);
		} finally {
			logsLoading = false;
		}
	}

	function toggleStreaming() {
		if (streaming) {
			stopStreaming();
		} else {
			startStreaming();
		}
	}

	function startStreaming() {
		const token = localStorage.getItem('token');
		if (!token) return;

		const hostId = localStorage.getItem('currentHostId') || '';
		eventSource = new EventSource(`/api/v1/sse/logs/${containerId}?token=${token}&host=${hostId}`);
		streaming = true;

		eventSource.addEventListener('log', (event) => {
			logs = [...logs, event.data];
			// Auto-scroll to bottom
			const container = document.getElementById('logs-container');
			if (container) {
				container.scrollTop = container.scrollHeight;
			}
		});

		eventSource.addEventListener('error', (event) => {
			console.error('SSE error:', event);
			stopStreaming();
		});
	}

	function stopStreaming() {
		if (eventSource) {
			eventSource.close();
			eventSource = null;
		}
		streaming = false;
	}

	async function startContainer() {
		await dockerApi.post(`/docker/containers/${containerId}/start`);
		await loadContainer();
	}

	async function stopContainer() {
		await dockerApi.post(`/docker/containers/${containerId}/stop`);
		await loadContainer();
	}

	async function restartContainer() {
		await dockerApi.post(`/docker/containers/${containerId}/restart`);
		await loadContainer();
	}

	async function inspectContainer() {
		try {
			inspectData = await dockerApi.get(`/docker/containers/${containerId}/inspect`);
			showInspect = true;
		} catch (e) {
			console.error('Failed to inspect container:', e);
		}
	}

	function getStateColor(state: string): string {
		switch (state) {
			case 'running': return 'bg-green-500';
			case 'exited': return 'bg-red-500';
			case 'paused': return 'bg-yellow-500';
			default: return 'bg-gray-500';
		}
	}
</script>

<div class="flex h-full flex-col p-6">
	<!-- Header -->
	<div class="mb-6 flex items-center gap-4">
		<Button variant="secondary" onclick={() => goto(resolve('/containers'))}>
			<ArrowLeft size={16} class="mr-2" />
			{tContainersBack}
		</Button>
		{#if container}
			<div class="flex items-center gap-3">
				<div class="h-3 w-3 rounded-full {getStateColor(container.state)}"></div>
				<h1 class="text-2xl font-semibold text-text-primary">{container.name}</h1>
			</div>
		{/if}
	</div>

	{#if loading}
		<Spinner size="lg" />
	{:else if container}
		<!-- Container Info -->
		<Card class="mb-6">
			<div class="grid grid-cols-2 gap-4 text-sm">
				<div><span class="text-text-secondary">ID:</span> <span class="text-text-primary">{container.id}</span></div>
				<div><span class="text-text-secondary">{tCommonStatus}</span> <span class="text-text-primary">{container.status}</span></div>
				<div class="col-span-2"><span class="text-text-secondary">{tContainersImage}</span> <span class="text-text-primary">{container.image}</span></div>
				{#if container.ports?.length > 0}
					<div class="col-span-2">
						<span class="text-text-secondary">{tCommonPort}</span>
						{#each container.ports as port}
							{#if port.hostPort}
								<a href="http://localhost:{port.hostPort}" target="_blank" class="ml-2 text-blue-400 hover:underline">{port.hostPort}:{port.containerPort}</a>
							{/if}
						{/each}
					</div>
				{/if}
			</div>
			<div class="mt-4 flex gap-2">
			{#if container.state === 'running'}
			<Button variant="secondary" size="sm" onclick={stopContainer}><StopCircle size={14} class="mr-1" />{tComposeStop}</Button>
			<Button variant="secondary" size="sm" onclick={restartContainer}><RefreshCw size={14} class="mr-1" />{tContainersRestart}</Button>
			<Button variant="secondary" size="sm" onclick={() => goto(resolve(`/containers/${containerId}/terminal`))}>
			<Terminal size={14} class="mr-1" />{tContainersTerminal}
			</Button>
			{:else}
			<Button variant="primary" size="sm" onclick={startContainer}><Play size={14} class="mr-1" />{tContainersStart}</Button>
			{/if}
			 <Button variant="secondary" size="sm" onclick={inspectContainer}><Info size={14} class="mr-1" />Inspect</Button>
				</div>
		</Card>

		<!-- Logs -->
		<Card>
			<div class="mb-4 flex items-center justify-between">
				<h2 class="text-lg font-semibold text-text-primary">{tComposeLogs}</h2>
				<div class="flex gap-2">
					<select bind:value={tail} class="rounded border border-border-secondary bg-surface-secondary px-2 py-1 text-sm text-text-primary">
						<option value={50}>50 {tContainersLine}</option>
						<option value={100}>100 {tContainersLine}</option>
						<option value={500}>500 {tContainersLine}</option>
						<option value={1000}>1000 {tContainersLine}</option>
					</select>
					<Button variant="secondary" size="sm" onclick={loadLogs}>
						<RefreshCw size={14} class={logsLoading ? 'animate-spin' : ''} />
					</Button>
					<Button
						variant={streaming ? 'danger' : 'secondary'}
						size="sm"
						onclick={toggleStreaming}
					>
						{streaming ? tContainersStopstream : tContainersLogsstream}
					</Button>
				</div>
			</div>
			<div id="logs-container" class="max-h-[500px] overflow-auto rounded bg-black p-4 font-mono text-xs text-green-400">
				{#if logsLoading}
					<Spinner size="sm" />
				{:else if logs.length === 0}
					<p class="text-text-muted">{tContainersNologs}</p>
				{:else}
					{#each logs as line}
						<div class="whitespace-pre-wrap">{line}</div>
					{/each}
				{/if}
			</div>
		</Card>
	{/if}

	<!-- Inspect Modal -->
	{#if showInspect && inspectData}
		<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
			<div class="mx-4 max-h-[80vh] w-full max-w-4xl overflow-auto rounded-lg bg-surface-primary p-6">
				<div class="mb-4 flex items-center justify-between">
					<h2 class="text-lg font-semibold text-text-primary">Container Inspect</h2>
					<Button variant="secondary" size="sm" onclick={() => showInspect = false}>{tCommonClose}</Button>
				</div>
				<pre class="max-h-[60vh] overflow-auto rounded bg-black p-4 font-mono text-xs text-green-400">{JSON.stringify(inspectData, null, 2)}</pre>
			</div>
		</div>
	{/if}
</div>
