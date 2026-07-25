<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { Spinner } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import { dockerApi } from '$lib/api/docker';
	import { Cpu, MemoryStick, Globe, Gauge, Container, Package, HardDrive, Activity } from 'lucide-svelte';
	import * as echarts from 'echarts';

	let stats = $state({
		containers: { total: 0, running: 0, stopped: 0, paused: 0 },
		compose: { total: 0, running: 0 },
		images: { total: 0, size: 0 }
	});

	let hostStats = $state({
		cpu: { usage: 0, cores: 0 },
		memory: { total: 0, used: 0, percent: 0 },
		network: { rx: 0, tx: 0 },
		load: { avg1: 0, avg5: 0, avg15: 0 }
	});

	let loading = $state(true);
	let eventSource: EventSource | null = null;
	let cpuChartEl: HTMLDivElement;
	let memChartEl: HTMLDivElement;
	let netChartEl: HTMLDivElement;
	let cpuChart: echarts.ECharts | null = null;
	let memChart: echarts.ECharts | null = null;
	let netChart: echarts.ECharts | null = null;

	const MAX_POINTS = 1800;
	const cpuHistory: { time: string; value: number }[] = [];
	const memHistory: { time: string; value: number }[] = [];
	const netRxHistory: { time: string; value: number }[] = [];
	const netTxHistory: { time: string; value: number }[] = [];
	let prevRx = 0;
	let prevTx = 0;

	onMount(async () => {
		window.addEventListener('host-changed', onHostChanged);
		await loadOverview();
		// Init charts after DOM renders (loading=false)
		requestAnimationFrame(() => { initCharts(); });
		connectSSE();
	});

	onDestroy(() => {
		window.removeEventListener('host-changed', onHostChanged);
		if (eventSource) eventSource.close();
		cpuChart?.dispose();
		memChart?.dispose();
		netChart?.dispose();
	});

	function onHostChanged() {
		if (eventSource) eventSource.close();
		loadOverview();
		connectSSE();
	}

	function formatTime(ts?: number): string {
		const d = ts ? new Date(ts) : new Date();
		return d.getHours().toString().padStart(2, '0') + ':' + d.getMinutes().toString().padStart(2, '0') + ':' + d.getSeconds().toString().padStart(2, '0');
	}

	function formatSize(bytes: number): string {
		if (!bytes) return '0 B';
		const k = 1024;
		const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
		const i = Math.floor(Math.log(bytes) / Math.log(k));
		return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
	}

	function formatSpeed(bytesPerSec: number): string {
		if (!bytesPerSec) return '0 B/s';
		const k = 1024;
		const sizes = ['B/s', 'KB/s', 'MB/s', 'GB/s'];
		const i = Math.floor(Math.log(bytesPerSec) / Math.log(k));
		return parseFloat((bytesPerSec / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
	}

	function getChartOption(title: string, color: string, max: number, formatter: string) {
		return {
			title: { text: title, textStyle: { color: '#aaa', fontSize: 12 }, left: 10, top: 5 },
			tooltip: { trigger: 'axis', backgroundColor: '#1e1e1e', borderColor: '#333', textStyle: { color: '#ccc', fontSize: 11 } },
			grid: { left: 45, right: 15, top: 30, bottom: 25 },
			xAxis: { type: 'category', data: [], axisLabel: { color: '#666', fontSize: 10 }, axisLine: { lineStyle: { color: '#333' } } },
			yAxis: { type: 'value', max, splitLine: { lineStyle: { color: '#222' } }, axisLabel: { color: '#666', fontSize: 10, formatter } },
			series: [{
				type: 'line', data: [], smooth: true, showSymbol: false,
				lineStyle: { color, width: 1.5 },
				areaStyle: { color: color + '20' },
				itemStyle: { color }
			}]
		};
	}

	function getMultiLineChartOption() {
		return {
			title: { text: '网络流量', textStyle: { color: '#aaa', fontSize: 12 }, left: 10, top: 5 },
			tooltip: { trigger: 'axis', backgroundColor: '#1e1e1e', borderColor: '#333', textStyle: { color: '#ccc', fontSize: 11 } },
			legend: { data: ['↓ RX', '↑ TX'], textStyle: { color: '#888', fontSize: 10 }, right: 10, top: 5 },
			grid: { left: 55, right: 15, top: 35, bottom: 25 },
			xAxis: { type: 'category', data: [], axisLabel: { color: '#666', fontSize: 10 }, axisLine: { lineStyle: { color: '#333' } } },
			yAxis: { type: 'value', splitLine: { lineStyle: { color: '#222' } }, axisLabel: { color: '#666', fontSize: 10, formatter: (v: number) => formatSpeed(v) } },
			series: [
				{ name: '↓ RX', type: 'line', data: [], smooth: true, showSymbol: false, lineStyle: { color: '#3b82f6', width: 1.5 }, areaStyle: { color: 'rgba(59,130,246,0.1)' }, itemStyle: { color: '#3b82f6' } },
				{ name: '↑ TX', type: 'line', data: [], smooth: true, showSymbol: false, lineStyle: { color: '#f97316', width: 1.5 }, areaStyle: { color: 'rgba(249,115,22,0.1)' }, itemStyle: { color: '#f97316' } }
			]
		};
	}

	function initCharts() {
		if (cpuChartEl) { cpuChart = echarts.init(cpuChartEl); cpuChart.setOption(getChartOption('CPU', '#3b82f6', 100, '{value}%')); }
		if (memChartEl) { memChart = echarts.init(memChartEl); memChart.setOption(getChartOption('内存', '#22c55e', 100, '{value}%')); }
		if (netChartEl) { netChart = echarts.init(netChartEl); netChart.setOption(getMultiLineChartOption()); }
		// Fill charts with history
		updateCharts();
	}

	// Load overview data (latest + 1h history) from backend collector
	async function loadOverview() {
		try {
			const data = await dockerApi.get<{ host: any; docker: any; history: any[] }>('/sse/overview');

			// Apply latest host stats
			if (data.host) {
				const h = data.host;
				hostStats.cpu.usage = Math.round(h.cpu || 0);
				hostStats.cpu.cores = h.cpuCores || 0;
				hostStats.memory.total = h.memTotal || 0;
				hostStats.memory.used = h.memUsed || 0;
				hostStats.memory.percent = Math.round(h.memPct || 0);
				hostStats.network.rx = h.netRx || 0;
				hostStats.network.tx = h.netTx || 0;
				hostStats.load.avg1 = h.load1 || 0;
				hostStats.load.avg5 = h.load5 || 0;
				hostStats.load.avg15 = h.load15 || 0;
			}

			// Apply latest docker stats
			if (data.docker) {
				const d = data.docker;
				stats.containers = { total: d.containersTotal || 0, running: d.containersRunning || 0, stopped: d.containersStopped || 0, paused: 0 };
				stats.compose = { total: d.composeTotal || 0, running: d.composeRunning || 0 };
				stats.images = { total: d.imagesTotal || 0, size: d.imagesSize || 0 };
			}

			// Fill history arrays from backend collector
			if (data.history && data.history.length > 0) {
				let lastRx = 0, lastTx = 0;
				for (const pt of data.history) {
					const t = formatTime(pt.ts);
					cpuHistory.push({ time: t, value: Math.round(pt.cpu || 0) });
					memHistory.push({ time: t, value: Math.round(pt.memPct || 0) });
					const rxSpeed = lastRx > 0 ? (pt.netRx || 0) - lastRx : 0;
					const txSpeed = lastTx > 0 ? (pt.netTx || 0) - lastTx : 0;
					netRxHistory.push({ time: t, value: rxSpeed });
					netTxHistory.push({ time: t, value: txSpeed });
					lastRx = pt.netRx || 0;
					lastTx = pt.netTx || 0;
				}
				prevRx = lastRx;
				prevTx = lastTx;
				// Trim to MAX_POINTS
				while (cpuHistory.length > MAX_POINTS) cpuHistory.shift();
				while (memHistory.length > MAX_POINTS) memHistory.shift();
				while (netRxHistory.length > MAX_POINTS) netRxHistory.shift();
				while (netTxHistory.length > MAX_POINTS) netTxHistory.shift();
			}
		} catch (e) { console.error('loadOverview failed:', e); }
		finally { loading = false; }
	}

	function connectSSE() {
		const token = localStorage.getItem('accessToken');
		if (!token) return;
		const hostId = localStorage.getItem('currentHostId') || '';
		eventSource = new EventSource(`/api/v1/sse/host?token=${token}&host=${hostId}`);
		eventSource.addEventListener('host', (event) => {
			try {
				const h = JSON.parse(event.data);
				// Update KPI cards
				hostStats.cpu.usage = Math.round(h.cpu || 0);
				hostStats.cpu.cores = h.cpuCores || 0;
				hostStats.memory.total = h.memTotal || 0;
				hostStats.memory.used = h.memUsed || 0;
				hostStats.memory.percent = Math.round(h.memPct || 0);
				hostStats.network.rx = h.netRx || 0;
				hostStats.network.tx = h.netTx || 0;
				hostStats.load.avg1 = h.load1 || 0;
				hostStats.load.avg5 = h.load5 || 0;
				hostStats.load.avg15 = h.load15 || 0;
				// Append to history
				const t = formatTime(h.ts);
				cpuHistory.push({ time: t, value: hostStats.cpu.usage });
				memHistory.push({ time: t, value: hostStats.memory.percent });
				const rxSpeed = prevRx > 0 ? (h.netRx || 0) - prevRx : 0;
				const txSpeed = prevTx > 0 ? (h.netTx || 0) - prevTx : 0;
				prevRx = h.netRx || 0;
				prevTx = h.netTx || 0;
				netRxHistory.push({ time: t, value: rxSpeed });
				netTxHistory.push({ time: t, value: txSpeed });
				if (cpuHistory.length > MAX_POINTS) cpuHistory.shift();
				if (memHistory.length > MAX_POINTS) memHistory.shift();
				if (netRxHistory.length > MAX_POINTS) netRxHistory.shift();
				if (netTxHistory.length > MAX_POINTS) netTxHistory.shift();
				updateCharts();
			} catch {}
		});
		eventSource.addEventListener('error', () => {
			setTimeout(() => { if (!eventSource || eventSource.readyState === EventSource.CLOSED) connectSSE(); }, 5000);
		});
	}

	function updateCharts() {
		if (cpuChart) {
			cpuChart.setOption({ xAxis: { data: cpuHistory.map(d => d.time) }, series: [{ data: cpuHistory.map(d => d.value) }] });
		}
		if (memChart) {
			memChart.setOption({ xAxis: { data: memHistory.map(d => d.time) }, series: [{ data: memHistory.map(d => d.value) }] });
		}
		if (netChart) {
			netChart.setOption({
				xAxis: { data: netRxHistory.map(d => d.time) },
				series: [{ data: netRxHistory.map(d => d.value) }, { data: netTxHistory.map(d => d.value) }]
			});
		}
	}
</script>

<div class="flex h-full flex-col bg-surface-primary overflow-hidden">
	<div class="flex-1 overflow-auto p-4">
		<h1 class="mb-4 text-base font-semibold text-text-primary">概览</h1>

		{#if loading}
			<div class="flex items-center justify-center py-12"><Spinner size="lg" /></div>
		{:else}
			<!-- Row 1: Host KPI -->
			<div class="mb-3 grid grid-cols-2 gap-3 lg:grid-cols-4">
				<div class="rounded-lg border border-border-secondary bg-surface-secondary p-3">
					<div class="flex items-center gap-2 mb-1">
						<Cpu size={16} class="text-blue-400" />
						<span class="text-xs text-text-muted">CPU</span>
					</div>
					<div class="text-xl font-bold text-text-primary">{hostStats.cpu.usage}%</div>
					<div class="text-[11px] text-text-muted">{hostStats.cpu.cores} 核心</div>
				</div>
				<div class="rounded-lg border border-border-secondary bg-surface-secondary p-3">
					<div class="flex items-center gap-2 mb-1">
						<MemoryStick size={16} class="text-green-400" />
						<span class="text-xs text-text-muted">内存</span>
					</div>
					<div class="text-xl font-bold text-text-primary">{hostStats.memory.percent}%</div>
					<div class="text-[11px] text-text-muted">{formatSize(hostStats.memory.used)} / {formatSize(hostStats.memory.total)}</div>
				</div>
				<div class="rounded-lg border border-border-secondary bg-surface-secondary p-3">
					<div class="flex items-center gap-2 mb-1">
						<Globe size={16} class="text-orange-400" />
						<span class="text-xs text-text-muted">网络</span>
					</div>
					<div class="text-xl font-bold text-text-primary">↓{formatSize(hostStats.network.rx)}</div>
					<div class="text-[11px] text-text-muted">↑{formatSize(hostStats.network.tx)}</div>
				</div>
				<div class="rounded-lg border border-border-secondary bg-surface-secondary p-3">
					<div class="flex items-center gap-2 mb-1">
						<Gauge size={16} class="text-purple-400" />
						<span class="text-xs text-text-muted">负载</span>
					</div>
					<div class="text-xl font-bold text-text-primary">{hostStats.load.avg1}</div>
					<div class="text-[11px] text-text-muted">5m {hostStats.load.avg5.toFixed(1)} · 15m {hostStats.load.avg15.toFixed(1)}</div>
				</div>
			</div>

			<!-- Row 2: Docker KPI -->
			<div class="mb-3 grid grid-cols-2 gap-3 lg:grid-cols-4">
				<div class="rounded-lg border border-border-secondary bg-surface-secondary p-3">
					<div class="flex items-center gap-2 mb-1">
						<Container size={16} class="text-blue-400" />
						<span class="text-xs text-text-muted">容器</span>
					</div>
					<div class="text-xl font-bold text-text-primary">{stats.containers.running}<span class="text-sm font-normal text-text-muted">/{stats.containers.total}</span></div>
					<div class="text-[11px] text-text-muted">运行中</div>
				</div>
				<div class="rounded-lg border border-border-secondary bg-surface-secondary p-3">
					<div class="flex items-center gap-2 mb-1">
						<Package size={16} class="text-green-400" />
						<span class="text-xs text-text-muted">Compose</span>
					</div>
					<div class="text-xl font-bold text-text-primary">{stats.compose.total}</div>
					<div class="text-[11px] text-text-muted">项目</div>
				</div>
				<div class="rounded-lg border border-border-secondary bg-surface-secondary p-3">
					<div class="flex items-center gap-2 mb-1">
						<HardDrive size={16} class="text-orange-400" />
						<span class="text-xs text-text-muted">镜像</span>
					</div>
					<div class="text-xl font-bold text-text-primary">{stats.images?.total || 0}</div>
					<div class="text-[11px] text-text-muted">{formatSize(stats.images?.size || 0)}</div>
				</div>
				<div class="rounded-lg border border-border-secondary bg-surface-secondary p-3">
					<div class="flex items-center gap-2 mb-1">
						<Activity size={16} class="text-red-400" />
						<span class="text-xs text-text-muted">已停止</span>
					</div>
					<div class="text-xl font-bold text-text-primary">{stats.containers.stopped}</div>
					<div class="text-[11px] text-text-muted">{stats.containers.paused} 暂停</div>
				</div>
			</div>

			<!-- Row 3: CPU + Mem Curves -->
			<div class="mb-3 grid grid-cols-1 gap-3 lg:grid-cols-2">
				<div class="rounded-lg border border-border-secondary bg-surface-secondary p-1">
					<div bind:this={cpuChartEl} class="h-[200px] w-full"></div>
				</div>
				<div class="rounded-lg border border-border-secondary bg-surface-secondary p-1">
					<div bind:this={memChartEl} class="h-[200px] w-full"></div>
				</div>
			</div>

			<!-- Row 4: Network Curve -->
			<div class="rounded-lg border border-border-secondary bg-surface-secondary p-1">
				<div bind:this={netChartEl} class="h-[200px] w-full"></div>
			</div>
		{/if}
	</div>
</div>
