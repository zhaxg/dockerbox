<script lang="ts">
import { t, setLocale, getLocale } from '$lib/i18n/index.svelte';
const tContainersRunning = $derived(t("containers.running"));
const tContainersStopped = $derived(t("containers.stopped"));
const tContainersPaused = $derived(t("containers.paused"));
const tContainersCreated = $derived(t("containers.created"));
const tContainersRestartconfirm = $derived(t("containers.restartConfirm"));
const tContainersStopconfirm = $derived(t("containers.stopConfirm"));
const tContainersNoresources = $derived(t("containers.noResources"));
const tContainersPrunefailed = $derived(t("containers.pruneFailed"));
const tContainersPruneconfirm = $derived(t("containers.pruneConfirm"));
const tContainersPruneunused = $derived(t("containers.pruneUnused"));
const tContainersPruneimages = $derived(t("containers.pruneImages"));
const tContainersNocontainers = $derived(t("containers.noContainers"));
const tContainersNomatch = $derived(t("containers.noMatch"));
const tCommonCancel = $derived(t("common.cancel"));
const tCommonConfirm = $derived(t("common.confirm"));
const tContainersLogs = $derived(t("containers.logs"));
const tContainersRestart = $derived(t("containers.restart"));
const tContainersSearch = $derived(t("containers.search"));
const tContainersStart = $derived(t("containers.start"));
const tContainersStop = $derived(t("containers.stop"));
const tContainersTerminal = $derived(t("containers.terminal"));
const tContainersInspect = $derived(t("containers.inspect"));
const tContainersTitle = $derived(t("containers.title"));
const tTableName = $derived(t("table.name"));
const tTableState = $derived(t("table.state"));
const tTableUptime = $derived(t("table.uptime"));
const tTableCpu = $derived(t("table.cpu"));
const tTableMem = $derived(t("table.mem"));
const tTableTrans = $derived(t("table.trans"));
const tTablePorts = $derived(t("table.ports"));
const tTableActions = $derived(t("table.actions"));
	import { onMount, onDestroy } from 'svelte';
	import { Spinner, Button, Badge } from '$lib/components/ui';
	import { hostsApi, type DockerHostsConfig } from '$lib/api/hosts';
	import { dockerApi } from '$lib/api/docker';
	import { toastStore } from '$lib/stores/toast.svelte';
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
		Maximize2,
		Minimize2,
		ChevronDown,
		ChevronUp,
		BrushCleaning
	} from 'lucide-svelte';
	import { Terminal as XTerminal } from '@xterm/xterm';
	import { FitAddon } from '@xterm/addon-fit';
	import { WebLinksAddon } from '@xterm/addon-web-links';
	import '@xterm/xterm/css/xterm.css';

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
	let hostsConfig = $state<DockerHostsConfig>({ default: '', hosts: {} });
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

	// Draggable modal state
	interface ModalDragState { x: number; y: number; maximized: boolean; dragging: boolean; offsetX: number; offsetY: number }
	function createModalDragState(): ModalDragState { return { x: 0, y: 0, maximized: false, dragging: false, offsetX: 0, offsetY: 0 }; }
	let logsDrag = $state(createModalDragState());
	let inspectDrag = $state(createModalDragState());
	let execDrag = $state(createModalDragState());
	let confirmDrag = $state(createModalDragState());

	function resetDrag(state: ModalDragState) { state.x = 0; state.y = 0; state.maximized = false; state.dragging = false; }

	function onDragHeader(e: MouseEvent, state: ModalDragState) {
		if (state.maximized) return;
		state.dragging = true;
		state.offsetX = e.clientX - state.x;
		state.offsetY = e.clientY - state.y;
		e.preventDefault();
	}

	function onDragMove(e: MouseEvent) {
		for (const s of [logsDrag, inspectDrag, execDrag, confirmDrag]) {
			if (s.dragging) {
				s.x = e.clientX - s.offsetX;
				s.y = e.clientY - s.offsetY;
			}
		}
	}

	function onDragEnd() {
		for (const s of [logsDrag, inspectDrag, execDrag, confirmDrag]) {
			s.dragging = false;
		}
	}

	function toggleMaximize(state: ModalDragState) {
		if (state.maximized) {
			state.x = 0; state.y = 0; state.maximized = false;
		} else {
			state.x = 0; state.y = 0; state.maximized = true;
		}
	}

	function modalStyle(state: ModalDragState): string {
		if (state.maximized) return '';
		return `transform: translate(${state.x}px, ${state.y}px)`;
	}

	let logsEventSource: EventSource | null = null;
	let execWs: WebSocket | null = null;
	let execTerminalEl: HTMLDivElement | null = $state(null);
	let xterm: XTerminal | null = null;
	let fitAddon: FitAddon | null = null;

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
		window.addEventListener('host-changed', onHostChanged);
		window.addEventListener('mousemove', onDragMove);
		window.addEventListener('mouseup', onDragEnd);
		await Promise.all([loadContainers(), loadHosts()]);
		connectSSE();
	});

	onDestroy(() => {
		window.removeEventListener('host-changed', onHostChanged);
		window.removeEventListener('mousemove', onDragMove);
		window.removeEventListener('mouseup', onDragEnd);
		if (eventSource) eventSource.close();
		closeLogsStream();
		closeExec();
	});

	function onHostChanged() {
		if (eventSource) eventSource.close();
		loadContainers();
		loadHosts();
		connectSSE();
	}

	function cleanupUnused() {
		showConfirm(tContainersPruneunused, tContainersPruneconfirm, async () => {
			try {
				const [imgResult, netResult] = await Promise.all([
					dockerApi.post<{ deleted: number; spaceMB: number; message: string }>('/docker/images/prune'),
					dockerApi.post<{ deleted: number; message: string }>('/docker/networks/prune')
				]);
				const parts: string[] = [];
				if (imgResult.deleted > 0) parts.push(imgResult.message);
				if (netResult.deleted > 0) parts.push(netResult.message);
				if (parts.length > 0) {
					toastStore.success(parts.join('，'));
				} else {
					toastStore.info(tContainersNoresources);
				}
				loadContainers();
			} catch (e) {
				toastStore.error(tContainersPrunefailed);
				console.error(e);
			}
		});
	}

	async function loadHosts() {
		try {
			hostsConfig = await hostsApi.list();
			if (!hostsConfig.hosts) hostsConfig.hosts = {};
		} catch (e) { console.error(e); }
	}

	const currentHost = $derived(
		hostsConfig.hosts?.[localStorage.getItem('currentHostId') || hostsConfig.default]
	);

	async function loadContainers() {
		loading = true;
		try {
			const data = await dockerApi.get<{ containers: ContainerInfo[] }>('/docker/containers');
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
		const hostId = localStorage.getItem('currentHostId') || '';
		eventSource = new EventSource(`/api/v1/sse/stats?token=${token}&host=${hostId}`);
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
		resetDrag(confirmDrag);
	}
	function closeConfirm() { confirmDialog.open = false; }

	// --- Actions (local state update) ---
	async function startContainer(id: string) {
		try {
			await dockerApi.post(`/docker/containers/${id}/start`);
			const idx = containers.findIndex((c) => c.id === id);
			if (idx !== -1) containers[idx] = { ...containers[idx], state: 'running', status: 'Up' };
		} catch (e) { console.error(e); }
	}

	function stopContainer(id: string) {
		showConfirm(tContainersStop, tContainersStopconfirm, async () => {
			try {
				await dockerApi.post(`/docker/containers/${id}/stop`);
				const idx = containers.findIndex((c) => c.id === id);
				if (idx !== -1) containers[idx] = { ...containers[idx], state: 'exited', status: 'Exited', cpu: 0, memory: { usage: 0, limit: 0, percent: 0 }, network: { rxBytes: 0, txBytes: 0 } };
			} catch (e) { console.error(e); }
		});
	}

	function restartContainer(id: string) {
		showConfirm(tContainersRestart, tContainersRestartconfirm, async () => {
			try {
				await dockerApi.post(`/docker/containers/${id}/restart`);
				const idx = containers.findIndex((c) => c.id === id);
				if (idx !== -1) containers[idx] = { ...containers[idx], state: 'running', status: 'Up' };
			} catch (e) { console.error(e); }
		});
	}

	// --- Logs Modal ---
	async function viewLogs(id: string, name: string) {
		logsModal = { open: true, id, name, content: '', loading: true, tail: 100, streaming: false };
		resetDrag(logsDrag);
		try {
			const data = await dockerApi.get<{ lines: string[] }>(`/docker/containers/${id}/logs?tail=100`);
			logsModal.content = (data.logs || []).join('\n');
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
			const hostId = localStorage.getItem('currentHostId') || '';
			logsEventSource = new EventSource(`/api/v1/sse/logs/${logsModal.id}?token=${token}&host=${hostId}`);
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
		resetDrag(inspectDrag);
		try {
			const data = await dockerApi.get<any>(`/docker/containers/${id}/inspect`);
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
		resetDrag(execDrag);
		// Init xterm after DOM update
		setTimeout(() => initXterm(id), 50);
	}

	function initXterm(id: string) {
		if (!execTerminalEl) return;
		xterm = new XTerminal({
			fontFamily: 'Menlo, Monaco, Consolas, monospace',
			fontSize: 13,
			theme: { background: '#000000', foreground: '#00ff00', cursor: '#00ff00' },
			cursorBlink: true,
			allowProposedApi: true
		});
		fitAddon = new FitAddon();
		xterm.loadAddon(fitAddon);
		xterm.loadAddon(new WebLinksAddon());
		xterm.open(execTerminalEl);
		fitAddon.fit();

		xterm.onData((data) => {
			if (execWs?.readyState === WebSocket.OPEN) {
				execWs.send(new TextEncoder().encode(data));
			}
		});

		const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		const token = localStorage.getItem('accessToken') || '';
		const hostId = localStorage.getItem('currentHostId') || '';
		execWs = new WebSocket(`${protocol}//${window.location.host}/api/v1/docker/containers/${id}/exec?hostId=${hostId}`);
		execWs.onopen = () => {
			execModal.connected = true;
			execWs?.send(JSON.stringify({ type: 'auth', token }));
		};
		execWs.onmessage = (event) => {
			if (event.data instanceof Blob) {
				event.data.arrayBuffer().then((buf) => {
					xterm?.write(new Uint8Array(buf));
				});
			} else if (event.data instanceof ArrayBuffer) {
				xterm?.write(new Uint8Array(event.data));
			} else {
				xterm?.write(event.data);
			}
		};
		execWs.onclose = () => { execModal.connected = false; };
		execWs.onerror = () => { execModal.connected = false; };
		// Handle resize
		const ro = new ResizeObserver(() => fitAddon?.fit());
		ro.observe(execTerminalEl);
	}

	

	function closeExec() {
		if (execWs) { execWs.close(); execWs = null; }
		if (xterm) { xterm.dispose(); xterm = null; }
		fitAddon = null;
		execModal.open = false;
		execTerminalEl = null;
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
			case 'running': return tContainersRunning;
			case 'exited': return tContainersStopped;
			case 'paused': return tContainersPaused;
			case 'created': return tContainersCreated;
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

	// Derive the host address for port links based on connection type
	function getHostAddress(): string {
		const host = currentHost;
		if (!host) return 'localhost';
		if (host.driver === 'ssh' && host.endpoint) {
			// endpoint format: "user@host:port" or "user@host"
			const atIdx = host.endpoint.indexOf('@');
			const hostPart = atIdx !== -1 ? host.endpoint.slice(atIdx + 1) : host.endpoint;
			// strip port if present
			const colonIdx = hostPart.lastIndexOf(':');
			return colonIdx !== -1 ? hostPart.slice(0, colonIdx) : hostPart;
		}
		// socket or tcp — use the browser's hostname (matches how user accessed the app)
		return typeof window !== 'undefined' ? window.location.hostname : 'localhost';
	}

	let derivedHostAddress = $derived(getHostAddress());

	const thClass = 'px-3 py-1.5 text-left text-[11px] font-medium uppercase tracking-wider text-text-muted border-b border-border-secondary select-none whitespace-nowrap';
	const tdClass = 'px-3 py-2 text-[13px] text-text-primary border-b border-border-secondary/50';

	// i18n
</script>

<div class="flex h-full flex-col bg-surface-primary">
	<div class="flex items-center justify-between border-b border-border-secondary px-4 py-3">
		<h1 class="text-base font-semibold text-text-primary">
			{tContainersTitle}
			{#if currentHost}<Badge>{currentHost.name}</Badge>{/if}
			<Badge>{filteredContainers.length}</Badge>
		</h1>
		<div class="flex items-center gap-2">
			<div class="relative">
				<Search size={12} class="absolute left-2.5 top-1/2 -translate-y-1/2 text-text-muted" />
				<input type="text" bind:value={searchQuery} placeholder={tContainersSearch}
					class="h-7 w-48 rounded border border-border-secondary bg-surface-secondary pl-8 pr-2 text-xs text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none" />
			</div>
			<Button variant="secondary" size="sm" onclick={cleanupUnused} title={tContainersPruneimages}><BrushCleaning size={12} /></Button>
			<Button variant="secondary" size="sm" onclick={loadContainers}><RefreshCw size={12} /></Button>
		</div>
	</div>

	<div class="flex-1 overflow-x-auto overflow-y-auto">
		{#if loading}
			<div class="flex items-center justify-center py-12"><Spinner size="lg" /></div>
		{:else if filteredContainers.length === 0}
			<div class="flex flex-col items-center gap-2 py-12 text-text-muted">
				<Container size={36} class="opacity-50" />
				<span class="text-sm">{searchQuery ? tContainersNomatch : tContainersNocontainers}</span>
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
						<th class="{thClass}">{tTableName}</th>
						<th class="{thClass}">{tTableState}</th>
						<th class="{thClass}">{tTableUptime}</th>
						<th class="{thClass} text-right">{tTableCpu}</th>
						<th class="{thClass} text-right">{tTableMem}</th>
						<th class="{thClass}">{tTableTrans}</th>
						<th class="{thClass}">{tTablePorts}</th>
						<th class="{thClass} text-right">{tTableActions}</th>
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
											<a href="http://{derivedHostAddress}:{port.hostPort}" target="_blank" rel="noopener noreferrer"
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
										<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-red-400 transition-colors hover:bg-red-500/10" onclick={() => stopContainer(container.id)} title={tContainersStop}>
											<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2"/></svg>
										</button>
										<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary" onclick={() => restartContainer(container.id)} title={tContainersRestart}>
											<RefreshCw size={13} />
										</button>
									{:else}
										<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-green-500 transition-colors hover:bg-green-500/10" onclick={() => startContainer(container.id)} title={tContainersStart}>
											<Play size={13} />
										</button>
									{/if}
									<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary" onclick={() => viewLogs(container.id, container.name)} title={tContainersLogs}>
										<Eye size={13} />
									</button>
									<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary" onclick={() => openExec(container.id, container.name)} title={tContainersTerminal}>
										<Terminal size={13} />
									</button>
									<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary" onclick={() => viewInspect(container.id, container.name)} title={tContainersInspect}>
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
		<div class="w-96 rounded-lg bg-surface-primary p-6 shadow-xl border border-border-secondary" style={modalStyle(confirmDrag)}>
			<h2 class="mb-2 text-lg font-semibold text-text-primary flex items-center gap-2">
				<span class="cursor-move" role="button" tabindex="-1" onmousedown={(e) => onDragHeader(e, confirmDrag)}>{confirmDialog.title}</span>
				<button type="button" class="ml-auto rounded p-1 text-text-secondary transition-colors hover:text-text-primary" onclick={() => toggleMaximize(confirmDrag)}>
					{#if confirmDrag.maximized}<Minimize2 size={12} />{:else}<Maximize2 size={12} />{/if}
				</button>
			</h2>
			<p class="mb-6 text-sm text-text-secondary">{confirmDialog.message}</p>
			<div class="flex justify-end gap-2">
				<Button variant="secondary" onclick={closeConfirm}>{tCommonCancel}</Button>
				<Button variant="danger" onclick={() => { confirmDialog.onConfirm(); closeConfirm(); }}>{tCommonConfirm}</Button>
			</div>
		</div>
	</div>
{/if}

<!-- Logs Modal -->
{#if logsModal.open}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-3">
		<div class="flex flex-col rounded-lg bg-surface-primary p-3 shadow-xl border border-border-secondary {logsDrag.maximized ? 'fixed inset-3' : 'h-[70vh] w-[750px]'}" style={modalStyle(logsDrag)}>
			<div class="flex items-center justify-between px-3 py-2 cursor-move" role="button" tabindex="-1" onmousedown={(e) => onDragHeader(e, logsDrag)}>
				<h3 class="text-sm font-semibold text-text-primary">{tContainersLogs} - {logsModal.name}</h3>
				<div class="flex items-center gap-1">
					<button type="button" class="rounded p-1 text-text-secondary transition-colors hover:text-text-primary" onclick={() => toggleMaximize(logsDrag)}>
						{#if logsDrag.maximized}<Minimize2 size={12} />{:else}<Maximize2 size={12} />{/if}
					</button>
					<button type="button" class="rounded p-1 text-text-secondary transition-colors hover:text-text-primary" onclick={() => { closeLogsStream(); logsModal.open = false; }}>
						<X size={16} />
					</button>
				</div>
			</div>
			<div class="flex-1 overflow-auto rounded-md bg-black p-3">
				{#if logsModal.loading}
					<div class="flex items-center justify-center py-8"><Spinner /></div>
				{:else}
					<pre class="whitespace-pre font-mono text-xs overflow-x-auto" style="color: #00ff00">{logsModal.content}</pre>
				{/if}
			</div>
		</div>
	</div>
{/if}

<!-- Inspect Modal -->
{#if inspectModal.open}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
		<div class="flex flex-col rounded-lg bg-surface-primary p-3 shadow-xl border border-border-secondary {inspectDrag.maximized ? 'fixed inset-3' : 'h-[80vh] w-[900px]'}" style={modalStyle(inspectDrag)}>
			<div class="flex items-center justify-between px-3 py-2 cursor-move" role="button" tabindex="-1" onmousedown={(e) => onDragHeader(e, inspectDrag)}>
				<div class="flex items-center gap-2">
					<h3 class="text-sm font-semibold text-text-primary">{tContainersInspect} - {inspectModal.name}</h3>
				</div>
				<div class="flex items-center gap-1">
					<button type="button" class="rounded p-1 text-text-secondary transition-colors hover:text-text-primary" onclick={() => toggleMaximize(inspectDrag)}>
						{#if inspectDrag.maximized}<Minimize2 size={12} />{:else}<Maximize2 size={12} />{/if}
					</button>
					<button type="button" class="rounded p-1 text-text-secondary transition-colors hover:text-text-primary" onclick={() => { inspectModal.open = false; }}>
						<X size={16} />
					</button>
				</div>
			</div>
			<div class="flex-1 overflow-auto rounded-md bg-black p-3">
				{#if inspectModal.loading}
					<div class="flex items-center justify-center py-8"><Spinner /></div>
				{:else}
					<pre class="whitespace-pre font-mono text-xs overflow-x-auto" style="color: #00ff00">{inspectModal.content}</pre>
				{/if}
			</div>
		</div>
	</div>
{/if}

<!-- Exec Modal -->
{#if execModal.open}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-3">
		<div class="flex flex-col rounded-lg bg-surface-primary p-3 shadow-xl border border-border-secondary overflow-hidden {execDrag.maximized ? 'fixed inset-3' : 'h-[70vh] w-[750px]'}" style={modalStyle(execDrag)}>
			<div class="flex items-center justify-between px-3 py-2 cursor-move" role="button" tabindex="-1" onmousedown={(e) => onDragHeader(e, execDrag)}>
				<h3 class="text-sm font-semibold text-text-primary">{tContainersTerminal} - {execModal.name}</h3>
				<div class="flex items-center gap-1">
					<button type="button" class="rounded p-1 text-text-secondary transition-colors hover:text-text-primary" onclick={() => toggleMaximize(execDrag)}>
						{#if execDrag.maximized}<Minimize2 size={12} />{:else}<Maximize2 size={12} />{/if}
					</button>
					<button type="button" class="rounded p-1 text-text-secondary transition-colors hover:text-text-primary" onclick={closeExec}>
						<X size={16} />
					</button>
				</div>
			</div>
			<div class="flex-1 min-h-0 rounded-md bg-black p-3">
				<div
					bind:this={execTerminalEl}
					class="h-full w-full focus:outline-none"
				></div>
			</div>
		</div>
	</div>
{/if}
