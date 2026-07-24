<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { Spinner, Button } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import {
		Play,
		StopCircle,
		RefreshCw,
		ExternalLink,
		Search,
		Container,
		Eye,
		Terminal,
		Info,
		X,
		ChevronDown,
		ChevronUp,
		BrushCleaning
	} from 'lucide-svelte';

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
		memory: { usage: number; limit: number; percent: number };
		network: { rxBytes: number; txBytes: number };
	}

	let containers = $state<ContainerInfo[]>([]);
	let loading = $state(true);
	let searchQuery = $state('');
	let eventSource: EventSource | null = null;

	// Modal states
	let logsModal = $state<{ open: boolean; id: string; name: string; content: string; loading: boolean; tail: number; streaming: boolean }>({
		open: false, id: '', name: '', content: '', loading: false, tail: 100, streaming: false
	});
	let inspectModal = $state<{ open: boolean; id: string; name: string; content: string; loading: boolean }>({
		open: false, id: '', name: '', content: '', loading: false
	});
	let execModal = $state<{ open: boolean; id: string; name: string; connected: boolean; output: string }>({
		open: false, id: '', name: '', connected: false, output: ''
	});
	let confirmDialog = $state<{ open: boolean; title: string; message: string; onConfirm: () => void }>({
		open: false, title: '', message: '', onConfirm: () => {}
	});
	let logsEventSource: EventSource | null = null;
	let execWs: WebSocket | null = null;
	let execInput = $state('');

	const filteredContainers = $derived(
		searchQuery.trim()
			? containers.filter(
					(c) =>
						c.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
						c.image.toLowerCase().includes(searchQuery.toLowerCase())
				)
			: containers
	);

	onMount(async () => {
		await loadContainers();
		connectSSE();
	});

	onDestroy(() => {
		if (eventSource) eventSource.close();
		closeLogsStream();
		closeExec();
	});

	function cleanupUnused() {
		showConfirm('清理未使用资源', '确定要清理所有未使用的镜像和网络吗？', async () => {
			try {
				await Promise.all([
					api.post('/docker/images/prune'),
					api.post('/docker/networks/prune')
				]);
			} catch (e) {
				console.error(e);
			}
		});
	}

	async function loadContainers() {
		loading = true;
		try {
			const data = await api.get<{ containers: ContainerInfo[] }>('/docker/containers');
			containers = data.containers || [];
		} catch (e) {
			console.error(e);
		} finally {
			loading = false;
		}
	}

	function connectSSE() {
		const token = localStorage.getItem('accessToken');
		if (!token) return;
		eventSource = new EventSource(`/api/v1/sse/stats?token=${token}`);
		eventSource.addEventListener('stats', (event) => {
			try {
				const data = JSON.parse(event.data);
				if (data.containers) {
					for (const update of data.containers) {
						const idx = containers.findIndex((c) => c.id === update.id);
						if (idx !== -1) {
							containers[idx] = {
								...containers[idx],
								cpu: update.cpu ?? containers[idx].cpu,
								memory: update.memory ?? containers[idx].memory,
								network: update.network ?? containers[idx].network
							};
						}
					}
				}
			} catch {}
		});
	}

	function showConfirm(title: string, message: string, onConfirm: () => void) {
		confirmDialog = { open: true, title, message, onConfirm };
	}
	function closeConfirm() { confirmDialog.open = false; }

	// --- Actions (local state update) ---
	async function startContainer(id: string) {
		try {
			await api.post(`/docker/containers/${id}/start`);
			const idx = containers.findIndex((c) => c.id === id);
			if (idx !== -1) containers[idx] = { ...containers[idx], state: 'running', status: 'Up' };
		} catch (e) { console.error(e); }
	}

	function stopContainer(id: string) {
		showConfirm('停止容器', '确定要停止这个容器吗？', async () => {
			try {
				await api.post(`/docker/containers/${id}/stop`);
				const idx = containers.findIndex((c) => c.id === id);
				if (idx !== -1) containers[idx] = { ...containers[idx], state: 'exited', status: 'Exited', cpu: 0, memory: { usage: 0, limit: 0, percent: 0 }, network: { rxBytes: 0, txBytes: 0 } };
			} catch (e) { console.error(e); }
		});
	}

	function restartContainer(id: string) {
		showConfirm('重启容器', '确定要重启这个容器吗？', async () => {
			try {
				await api.post(`/docker/containers/${id}/restart`);
				const idx = containers.findIndex((c) => c.id === id);
				if (idx !== -1) containers[idx] = { ...containers[idx], state: 'running', status: 'Up' };
			} catch (e) { console.error(e); }
		});
	}

	// --- Logs Modal ---
	async function viewLogs(id: string, name: string) {
		logsModal = { open: true, id, name, content: '', loading: true, tail: 100, streaming: false };
		try {
			const data = await api.get<{ lines: string[] }>(`/docker/containers/${id}/logs?tail=100`);
			logsModal.content = (data.lines || []).join('\n');
		} catch {
			logsModal.content = 'Failed to load logs';
		} finally {
			logsModal.loading = false;
		}
	}

	function toggleLogsStream() {
		if (logsModal.streaming) {
			closeLogsStream();
		} else {
			const token = localStorage.getItem('accessToken');
			if (!token) return;
			logsEventSource = new EventSource(`/api/v1/sse/logs/${logsModal.id}?token=${token}`);
			logsModal.streaming = true;
			logsEventSource.addEventListener('log', (event) => {
				logsModal.content += '\n' + event.data;
				const el = document.getElementById('logs-pre');
				if (el) el.scrollTop = el.scrollHeight;
			});
			logsEventSource.addEventListener('error', () => closeLogsStream());
		}
	}

	function closeLogsStream() {
		if (logsEventSource) { logsEventSource.close(); logsEventSource = null; }
		logsModal.streaming = false;
	}

	// --- Inspect Modal ---
	async function viewInspect(id: string, name: string) {
		inspectModal = { open: true, id, name, content: '', loading: true };
		try {
			const data = await api.get<any>(`/docker/containers/${id}/inspect`);
			inspectModal.content = JSON.stringify(data, null, 2);
		} catch {
			inspectModal.content = 'Failed to inspect container';
		} finally {
			inspectModal.loading = false;
		}
	}

	// --- Exec Modal ---
	function openExec(id: string, name: string) {
		execModal = { open: true, id, name, connected: false, output: '' };
		const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		const token = localStorage.getItem('accessToken') || '';
		execWs = new WebSocket(`${protocol}//${window.location.host}/api/v1/docker/containers/${id}/exec`);
		execWs.onopen = () => {
			execModal.connected = true;
			execModal.output = '已连接到容器终端...\r\n';
			execWs?.send(JSON.stringify({ type: 'auth', token }));
		};
		execWs.onmessage = (event) => {
			if (typeof event.data === 'string') {
				try {
					const msg = JSON.parse(event.data);
					if (msg.type === 'output') execModal.output += atob(msg.data);
				} catch {
					execModal.output += event.data;
				}
			} else if (event.data instanceof Blob) {
				event.data.arrayBuffer().then((buf) => {
					execModal.output += new TextDecoder().decode(buf);
					scrollExec();
				});
			}
		};
		execWs.onclose = () => { execModal.connected = false; };
		execWs.onerror = () => { execModal.connected = false; };
	}

	function sendExecInput(e: KeyboardEvent) {
		if (e.key === 'Enter' && execWs?.readyState === WebSocket.OPEN) {
			execWs.send(JSON.stringify({ type: 'input', data: execInput + '\r' }));
			execModal.output += execInput + '\r\n';
			execInput = '';
		}
	}

	function scrollExec() {
		const el = document.getElementById('exec-pre');
		if (el) el.scrollTop = el.scrollHeight;
	}

	function closeExec() {
		if (execWs) { execWs.close(); execWs = null; }
		execModal.open = false;
	}

	// --- Helpers ---
	function getStateColor(state: string): string {
		switch (state) {
			case 'running': return 'bg-green-500';
			case 'exited': return 'bg-red-500';
			case 'paused': return 'bg-yellow-500';
			default: return 'bg-gray-500';
		}
	}
	function getStateText(state: string): string {
		switch (state) {
			case 'running': return '运行中';
			case 'exited': return '已停止';
			case 'paused': return '已暂停';
			case 'created': return '已创建';
			default: return state;
		}
	}
	function formatUptime(created: string): string {
		const diff = Date.now() - new Date(created).getTime();
		const d = Math.floor(diff / 86400000);
		const h = Math.floor((diff % 86400000) / 3600000);
		const m = Math.floor((diff % 3600000) / 60000);
		if (d > 0) return `${d}d ${h}h`;
		if (h > 0) return `${h}h ${m}m`;
		return `${m}m`;
	}
	function formatBytes(bytes: number): string {
		if (!bytes) return '0 B';
		const k = 1024;
		const sizes = ['B', 'KB', 'MB', 'GB'];
		const i = Math.floor(Math.log(bytes) / Math.log(k));
		return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
	}
	function formatTraffic(rx: number, tx: number): string {
		if (!rx && !tx) return '-';
		return `↓${formatBytes(rx)} ↑${formatBytes(tx)}`;
	}
	let hostIp = $state('localhost');
	onMount(async () => {
		try {
			const data = await api.get<{ ip: string }>('/docker/containers/host-ip');
			hostIp = data.ip || 'localhost';
		} catch {}
	});

	const thClass = 'px-3 py-1.5 text-left text-[11px] font-medium uppercase tracking-wider text-text-muted border-b border-border-secondary select-none whitespace-nowrap';
	const tdClass = 'px-3 py-2 text-[13px] text-text-primary border-b border-border-secondary/50';
</script>

<div class="flex h-full flex-col bg-surface-primary">
	<div class="flex items-center justify-between border-b border-border-secondary px-4 py-3">
		<h1 class="text-base font-semibold text-text-primary">
			容器 <span class="ml-1 text-sm font-normal text-text-muted">({filteredContainers.length})</span>
		</h1>
		<div class="flex items-center gap-2">
			<div class="relative">
				<Search size={14} class="absolute left-2.5 top-1/2 -translate-y-1/2 text-text-muted" />
				<input type="text" bind:value={searchQuery} placeholder="搜索容器..."
					class="h-7 w-48 rounded border border-border-secondary bg-surface-secondary pl-8 pr-2 text-xs text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none" />
			</div>
			<Button variant="secondary" size="sm" onclick={cleanupUnused} title="清理未使用的镜像和网络"><BrushCleaning size={14} /></Button>
			<Button variant="secondary" size="sm" onclick={loadContainers}><RefreshCw size={14} /></Button>
		</div>
	</div>

	<div class="flex-1 overflow-x-auto overflow-y-auto">
		{#if loading}
			<div class="flex items-center justify-center py-12"><Spinner size="lg" /></div>
		{:else if filteredContainers.length === 0}
			<div class="flex flex-col items-center gap-2 py-12 text-text-muted">
				<Container size={36} class="opacity-50" />
				<span class="text-sm">{searchQuery ? '没有匹配的容器' : '暂无容器'}</span>
			</div>
		{:else}
			<table class="w-full min-w-[1050px] border-collapse text-[13px] leading-5">
				<colgroup>
					<col class="w-[200px]" />
					<col class="w-[80px]" />
					<col class="w-[80px]" />
					<col class="w-[70px]" />
					<col class="w-[90px]" />
					<col class="w-[180px]" />
					<col />
					<col class="w-[160px]" />
				</colgroup>
				<thead>
					<tr>
						<th class="{thClass}">Name</th>
						<th class="{thClass}">State</th>
						<th class="{thClass}">Uptime</th>
						<th class="{thClass} text-right">CPU</th>
						<th class="{thClass} text-right">Mem</th>
						<th class="{thClass}">Trans</th>
						<th class="{thClass}">Ports</th>
						<th class="{thClass} text-right">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each filteredContainers as container (container.id)}
						<tr class="transition-colors hover:bg-surface-secondary">
							<td class="{tdClass}">
								<div class="flex items-center gap-2">
									<span class="h-2 w-2 shrink-0 rounded-full {getStateColor(container.state)}"></span>
									<span class="block truncate font-medium" title={container.name}>{container.name}</span>
								</div>
							</td>
							<td class="{tdClass}">
								<span class="inline-flex items-center rounded px-1.5 py-0.5 text-[11px] font-medium {container.state === 'running' ? 'bg-green-500/15 text-green-500' : container.state === 'exited' ? 'bg-red-500/15 text-red-500' : 'bg-gray-500/15 text-gray-400'}">
									{getStateText(container.state)}
								</span>
							</td>
							<td class="{tdClass} text-text-secondary tabular-nums">
								{container.state === 'running' ? formatUptime(container.created) : '-'}
							</td>
							<td class="{tdClass} text-right tabular-nums text-text-secondary">
								{container.cpu?.toFixed(1) || '0.0'}%
							</td>
							<td class="{tdClass} text-right tabular-nums text-text-secondary">
								{formatBytes(container.memory?.usage || 0)}
							</td>
							<td class="{tdClass} text-text-secondary tabular-nums text-[12px]">
								{formatTraffic(container.network?.rxBytes || 0, container.network?.txBytes || 0)}
							</td>
							<td class="{tdClass}">
								{#if [...new Map((container.ports || []).filter((p) => p.hostPort && p.hostPort !== '0').map((p) => [p.hostPort, p])).values()].length > 0}
									<div class="flex flex-wrap gap-1">
										{#each [...new Map((container.ports || []).filter((p) => p.hostPort && p.hostPort !== '0').map((p) => [p.hostPort, p])).values()].slice(0, 5) as port}
											<a href="http://{hostIp}:{port.hostPort}" target="_blank" rel="noopener noreferrer"
												class="inline-flex items-center gap-0.5 rounded bg-blue-500/10 px-1.5 py-0.5 text-[11px] text-blue-400 hover:bg-blue-500/20"
												title="{port.hostPort}:{port.containerPort}/{port.protocol}">
												{port.hostPort}
												<ExternalLink size={10} />
											</a>
										{/each}
										{#if [...new Map((container.ports || []).filter((p) => p.hostPort && p.hostPort !== '0').map((p) => [p.hostPort, p])).values()].length > 5}
											<span class="text-[11px] text-text-muted">+{[...new Map((container.ports || []).filter((p) => p.hostPort && p.hostPort !== '0').map((p) => [p.hostPort, p])).values()].length - 5}</span>
										{/if}
									</div>
								{/if}
							</td>
							<td class="{tdClass}">
								<div class="flex justify-end gap-1">
									{#if container.state === 'running'}
										<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-red-400 transition-colors hover:bg-red-500/10" onclick={() => stopContainer(container.id)} title="停止">
											<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2"/></svg>
										</button>
										<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary" onclick={() => restartContainer(container.id)} title="重启">
											<RefreshCw size={13} />
										</button>
									{:else}
										<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-green-500 transition-colors hover:bg-green-500/10" onclick={() => startContainer(container.id)} title="启动">
											<Play size={13} />
										</button>
									{/if}
									<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary" onclick={() => viewLogs(container.id, container.name)} title="日志">
										<Eye size={13} />
									</button>
									<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary" onclick={() => openExec(container.id, container.name)} title="终端">
										<Terminal size={13} />
									</button>
									<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary" onclick={() => viewInspect(container.id, container.name)} title="Inspect">
										<Info size={13} />
									</button>
								</div>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</div>
</div>

<!-- Confirm Dialog -->
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

<!-- Logs Modal -->
{#if logsModal.open}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
		<div class="flex h-[80vh] w-[900px] flex-col rounded-lg bg-surface-primary shadow-xl border border-border-secondary">
			<div class="flex items-center justify-between border-b border-border-secondary px-4 py-3">
				<div class="flex items-center gap-3">
					<h3 class="text-sm font-semibold text-text-primary">日志 - {logsModal.name}</h3>
					<select bind:value={logsModal.tail} onchange={() => viewLogs(logsModal.id, logsModal.name)}
						class="rounded border border-border-secondary bg-surface-secondary px-2 py-0.5 text-xs text-text-primary">
						<option value={50}>50 行</option>
						<option value={100}>100 行</option>
						<option value={500}>500 行</option>
						<option value={1000}>1000 行</option>
					</select>
					<button type="button" class="inline-flex items-center gap-1 rounded px-2 py-0.5 text-xs {logsModal.streaming ? 'bg-red-500/15 text-red-400' : 'bg-green-500/15 text-green-400'}"
						onclick={toggleLogsStream}>
						{#if logsModal.streaming}<ChevronDown size={12} /> 停止流{:else}<ChevronUp size={12} /> 实时流{/if}
					</button>
				</div>
				<button type="button" class="text-text-muted hover:text-text-primary" onclick={() => { closeLogsStream(); logsModal.open = false; }}>
					<X size={16} />
				</button>
			</div>
			<div class="flex-1 overflow-auto p-4">
				{#if logsModal.loading}
					<div class="flex items-center justify-center py-8"><Spinner /></div>
				{:else}
					<pre id="logs-pre" class="whitespace-pre-wrap font-mono text-xs text-text-secondary">{logsModal.content}</pre>
				{/if}
			</div>
		</div>
	</div>
{/if}

<!-- Inspect Modal -->
{#if inspectModal.open}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
		<div class="flex h-[80vh] w-[900px] flex-col rounded-lg bg-surface-primary shadow-xl border border-border-secondary">
			<div class="flex items-center justify-between border-b border-border-secondary px-4 py-3">
				<h3 class="text-sm font-semibold text-text-primary">Inspect - {inspectModal.name}</h3>
				<button type="button" class="text-text-muted hover:text-text-primary" onclick={() => { inspectModal.open = false; }}>
					<X size={16} />
				</button>
			</div>
			<div class="flex-1 overflow-auto p-4">
				{#if inspectModal.loading}
					<div class="flex items-center justify-center py-8"><Spinner /></div>
				{:else}
					<pre class="whitespace-pre-wrap font-mono text-xs text-text-secondary">{inspectModal.content}</pre>
				{/if}
			</div>
		</div>
	</div>
{/if}

<!-- Exec Modal -->
{#if execModal.open}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
		<div class="flex h-[80vh] w-[900px] flex-col rounded-lg bg-surface-primary shadow-xl border border-border-secondary">
			<div class="flex items-center justify-between border-b border-border-secondary px-4 py-3">
				<div class="flex items-center gap-2">
					<h3 class="text-sm font-semibold text-text-primary">终端 - {execModal.name}</h3>
					<span class="h-2 w-2 rounded-full {execModal.connected ? 'bg-green-500' : 'bg-red-500'}"></span>
				</div>
				<button type="button" class="text-text-muted hover:text-text-primary" onclick={closeExec}>
					<X size={16} />
				</button>
			</div>
			<div class="flex-1 overflow-auto bg-black p-4">
				<pre id="exec-pre" class="font-mono text-xs text-green-400 whitespace-pre-wrap">{execModal.output}</pre>
			</div>
			<div class="border-t border-border-secondary px-4 py-2">
				<input type="text" bind:value={execInput} onkeydown={sendExecInput}
					placeholder="输入命令..." disabled={!execModal.connected}
					class="w-full rounded border border-border-secondary bg-surface-secondary px-3 py-1.5 font-mono text-xs text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none" />
			</div>
		</div>
	</div>
{/if}
