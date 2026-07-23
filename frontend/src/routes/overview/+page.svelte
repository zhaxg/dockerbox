<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { Card, Spinner } from '$lib/components/ui';
	import { Container, Package, HardDrive, Activity, Cpu, MemoryStick, Globe, Gauge } from 'lucide-svelte';
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
	let cpuChart: echarts.ECharts | null = null;
	let memChart: echarts.ECharts | null = null;

	const MAX_POINTS = 30;
	const cpuHistory: { time: string; value: number }[] = [];
	const memHistory: { time: string; value: number }[] = [];

	onMount(async () => {
		await loadStats();
		connectSSE();
	});

	onDestroy(() => {
		if (eventSource) eventSource.close();
		cpuChart?.dispose();
		memChart?.dispose();
	});

	function formatTime(): string {
		const d = new Date();
		return d.getHours().toString().padStart(2, '0') + ':' + d.getMinutes().toString().padStart(2, '0') + ':' + d.getSeconds().toString().padStart(2, '0');
	}

	function initCpuChart() {
		if (!cpuChartEl) return;
		cpuChart?.dispose();
		cpuChart = echarts.init(cpuChartEl);
		cpuChart.setOption({
			title: { text: 'CPU 使用率', textStyle: { color: '#ccc', fontSize: 13 }, left: 'center' },
			tooltip: { trigger: 'axis' },
			grid: { left: 45, right: 10, top: 30, bottom: 20 },
			xAxis: { type: 'category', data: cpuHistory.map(d => d.time), axisLabel: { color: '#888', fontSize: 10 } },
			yAxis: { type: 'value', max: 100, axisLabel: { color: '#888', fontSize: 10, formatter: '{value}%' } },
			series: [{
				type: 'line',
				data: cpuHistory.map(d => d.value),
				smooth: true,
				lineStyle: { color: '#3b82f6', width: 2 },
				areaStyle: { color: 'rgba(59,130,246,0.15)' },
				itemStyle: { color: '#3b82f6' },
				showSymbol: false
			}]
		});
	}

	function initMemChart() {
		if (!memChartEl) return;
		memChart?.dispose();
		memChart = echarts.init(memChartEl);
		memChart.setOption({
			title: { text: '内存使用率', textStyle: { color: '#ccc', fontSize: 13 }, left: 'center' },
			tooltip: { trigger: 'axis' },
			grid: { left: 45, right: 10, top: 30, bottom: 20 },
			xAxis: { type: 'category', data: memHistory.map(d => d.time), axisLabel: { color: '#888', fontSize: 10 } },
			yAxis: { type: 'value', max: 100, axisLabel: { color: '#888', fontSize: 10, formatter: '{value}%' } },
			series: [{
				type: 'line',
				data: memHistory.map(d => d.value),
				smooth: true,
				lineStyle: { color: '#22c55e', width: 2 },
				areaStyle: { color: 'rgba(34,197,94,0.15)' },
				itemStyle: { color: '#22c55e' },
				showSymbol: false
			}]
		});
	}

	async function loadStats() {
		try {
			const token = localStorage.getItem('accessToken') || localStorage.getItem('token') || '';
			const headers = { 'Authorization': 'Bearer ' + token };
			const [statsData, composeData] = await Promise.all([
				fetch('/api/v1/docker/containers/stats', { headers }).then(r => {
					if (!r.ok) throw new Error('Stats API error');
					return r.json();
				}).catch(() => ({ containers: { total: 0, running: 0, stopped: 0, paused: 0 }, images: { total: 0, size: 0 } })),
				fetch('/api/v1/docker/compose', { headers }).then(r => r.json()).catch(() => ({ projects: [] }))
			]);
			stats = { ...statsData, compose: { total: composeData.projects?.length || 0, running: 0 } };
		} catch (e) {
			console.error('Failed to load stats:', e);
		} finally {
			loading = false;
		}
	}

	function connectSSE() {
		const token = localStorage.getItem('accessToken') || localStorage.getItem('token');
		if (!token) return;

		eventSource = new EventSource(`/api/v1/sse/host?token=${token}`);

		eventSource.addEventListener('host', (event) => {
			try {
				const data = JSON.parse(event.data);
				parseHostStats(data);
			} catch (e) {
				console.error('Failed to parse host stats:', e);
			}
		});

		eventSource.addEventListener('error', () => {});
	}

	function parseHostStats(data: any) {
		const t = formatTime();

		if (data.cpu?.raw) {
			const lines = data.cpu.raw.split('\n');
			const cpuLine = lines.find((l: string) => l.startsWith('cpu '));
			if (cpuLine) {
				const parts = cpuLine.split(/\s+/).slice(1).map(Number);
				const idle = parts[3] || 0;
				const total = parts.reduce((a: number, b: number) => a + b, 0);
				hostStats.cpu.usage = total > 0 ? Math.round(((total - idle) / total) * 100) : 0;
				hostStats.cpu.cores = lines.filter((l: string) => l.startsWith('cpu')).length;
			}
			cpuHistory.push({ time: t, value: hostStats.cpu.usage });
			if (cpuHistory.length > MAX_POINTS) cpuHistory.shift();
		}

		if (data.memory?.raw) {
			const lines = data.memory.raw.split('\n');
			const memTotal = lines.find((l: string) => l.startsWith('MemTotal:'));
			const memAvail = lines.find((l: string) => l.startsWith('MemAvailable:'));
			if (memTotal && memAvail) {
				const total = parseInt(memTotal.split(/\s+/)[1]) * 1024;
				const avail = parseInt(memAvail.split(/\s+/)[1]) * 1024;
				hostStats.memory.total = total;
				hostStats.memory.used = total - avail;
				hostStats.memory.percent = Math.round(((total - avail) / total) * 100);
			}
			memHistory.push({ time: t, value: hostStats.memory.percent });
			if (memHistory.length > MAX_POINTS) memHistory.shift();
		}

		if (data.network?.raw) {
			const lines = data.network.raw.split('\n');
			let rx = 0, tx = 0;
			for (const line of lines) {
				if (line.includes(':') && !line.startsWith('Inter') && !line.startsWith('face')) {
					const parts = line.split(':')[1]?.trim().split(/\s+/).map(Number) || [];
					rx += parts[0] || 0;
					tx += parts[8] || 0;
				}
			}
			hostStats.network.rx = rx;
			hostStats.network.tx = tx;
		}

		if (data.load?.raw) {
			const parts = data.load.raw.split(/\s+/);
			hostStats.load.avg1 = parseFloat(parts[0]) || 0;
			hostStats.load.avg5 = parseFloat(parts[1]) || 0;
			hostStats.load.avg15 = parseFloat(parts[2]) || 0;
		}

		// Update charts
		if (cpuChart) {
			cpuChart.setOption({
				xAxis: { data: cpuHistory.map(d => d.time) },
				series: [{ data: cpuHistory.map(d => d.value) }]
			});
		} else {
			initCpuChart();
		}
		if (memChart) {
			memChart.setOption({
				xAxis: { data: memHistory.map(d => d.time) },
				series: [{ data: memHistory.map(d => d.value) }]
			});
		} else {
			initMemChart();
		}
	}

	function formatSize(bytes: number): string {
		if (bytes === 0) return '0 B';
		const k = 1024;
		const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
		const i = Math.floor(Math.log(bytes) / Math.log(k));
		return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
	}

	function formatNetwork(bytes: number): string {
		if (bytes === 0) return '0 B';
		const k = 1024;
		const sizes = ['B', 'KB', 'MB', 'GB'];
		const i = Math.floor(Math.log(bytes) / Math.log(k));
		return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
	}
</script>

<div class="p-6">
	<h1 class="mb-6 text-2xl font-semibold text-text-primary">概览</h1>

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<Spinner size="lg" />
		</div>
	{:else}
		<!-- Host Stats Cards -->
		<div class="mb-6 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
			<Card>
				<div class="flex items-center gap-3">
					<div class="rounded-lg bg-blue-500/10 p-2">
						<Cpu size={24} class="text-blue-500" />
					</div>
					<div>
						<div class="text-2xl font-bold text-text-primary">{hostStats.cpu.usage}%</div>
						<div class="text-sm text-text-secondary">CPU ({hostStats.cpu.cores} 核心)</div>
					</div>
				</div>
			</Card>
			<Card>
				<div class="flex items-center gap-3">
					<div class="rounded-lg bg-green-500/10 p-2">
						<MemoryStick size={24} class="text-green-500" />
					</div>
					<div>
						<div class="text-2xl font-bold text-text-primary">{hostStats.memory.percent}%</div>
						<div class="text-sm text-text-secondary">内存 ({formatSize(hostStats.memory.used)} / {formatSize(hostStats.memory.total)})</div>
					</div>
				</div>
			</Card>
			<Card>
				<div class="flex items-center gap-3">
					<div class="rounded-lg bg-orange-500/10 p-2">
						<Globe size={24} class="text-orange-500" />
					</div>
					<div>
						<div class="text-2xl font-bold text-text-primary">↓{formatNetwork(hostStats.network.rx)}</div>
						<div class="text-sm text-text-secondary">↑{formatNetwork(hostStats.network.tx)}</div>
					</div>
				</div>
			</Card>
			<Card>
				<div class="flex items-center gap-3">
					<div class="rounded-lg bg-purple-500/10 p-2">
						<Gauge size={24} class="text-purple-500" />
					</div>
					<div>
						<div class="text-2xl font-bold text-text-primary">{hostStats.load.avg1}</div>
						<div class="text-sm text-text-secondary">负载 (5m: {hostStats.load.avg5.toFixed(1)}, 15m: {hostStats.load.avg15.toFixed(1)})</div>
					</div>
				</div>
			</Card>
		</div>

		<!-- Trend Charts -->
		<div class="mb-6 grid grid-cols-1 gap-4 lg:grid-cols-2">
			<Card>
				<div bind:this={cpuChartEl} class="h-[240px] w-full"></div>
			</Card>
			<Card>
				<div bind:this={memChartEl} class="h-[240px] w-full"></div>
			</Card>
		</div>

		<!-- Docker Stats -->
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
			<Card>
				<div class="flex items-center gap-3">
					<div class="rounded-lg bg-blue-500/10 p-2">
						<Container size={24} class="text-blue-500" />
					</div>
					<div>
						<div class="text-2xl font-bold text-text-primary">{stats.containers.running}</div>
						<div class="text-sm text-text-secondary">运行中容器 ({stats.containers.total} 总计)</div>
					</div>
				</div>
			</Card>
			<Card>
				<div class="flex items-center gap-3">
					<div class="rounded-lg bg-green-500/10 p-2">
						<Package size={24} class="text-green-500" />
					</div>
					<div>
						<div class="text-2xl font-bold text-text-primary">{stats.compose.total}</div>
						<div class="text-sm text-text-secondary">Compose 项目</div>
					</div>
				</div>
			</Card>
			<Card>
				<div class="flex items-center gap-3">
					<div class="rounded-lg bg-orange-500/10 p-2">
						<HardDrive size={24} class="text-orange-500" />
					</div>
					<div>
						<div class="text-2xl font-bold text-text-primary">{stats.images.total}</div>
						<div class="text-sm text-text-secondary">镜像 ({formatSize(stats.images.size)})</div>
					</div>
				</div>
			</Card>
			<Card>
				<div class="flex items-center gap-3">
					<div class="rounded-lg bg-purple-500/10 p-2">
						<Activity size={24} class="text-purple-500" />
					</div>
					<div>
						<div class="text-2xl font-bold text-text-primary">{stats.containers.stopped}</div>
						<div class="text-sm text-text-secondary">已停止 ({stats.containers.paused} 暂停)</div>
					</div>
				</div>
			</Card>
		</div>
	{/if}
</div>