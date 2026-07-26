<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { Spinner, Button, Badge } from '$lib/components/ui';
	import LogModal from '$lib/components/LogModal.svelte';
	import { hostsApi, type DockerHostsConfig } from '$lib/api/hosts';
	import { api } from '$lib/api/client';
	import { dockerApi } from '$lib/api/docker';
	import { Play, RefreshCw, Eye, RotateCcw, Package, Plus, Search, Trash2, X, Save, Download, BrushCleaning } from 'lucide-svelte';
	import type * as Monaco from 'monaco-editor';

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
	let searchQuery = $state('');
	let hostsConfig = $state<DockerHostsConfig>({ default: '', hosts: {} });

	// Confirm dialog
	let confirmDialog = $state<{ open: boolean; title: string; message: string; onConfirm: () => void }>({
		open: false, title: '', message: '', onConfirm: () => {}
	});

	// Deploy log modal
	let deployLog = $state<{ open: boolean; loading: boolean; content: string; name: string; projectId: string }>({
		open: false, loading: false, content: '', name: '', projectId: ''
	});

	// Edit/Create modal
	let editorModal = $state<{
		open: boolean;
		mode: 'new' | 'edit';
		projectId: string;
		projectName: string;
		projectPath: string;
		composeContent: string;
		saving: boolean;
		error: string;
		dirty: boolean;
		loading: boolean;
	}>({
		open: false, mode: 'new', projectId: '', projectName: '', projectPath: '',
		composeContent: '', saving: false, error: '', dirty: false, loading: false
	});

	// Import modal
	let importModal = $state<{ open: boolean; loading: boolean; projects: ComposeProject[]; selected: Set<string>; importing: boolean }>({
		open: false, loading: false, projects: [], selected: new Set(), importing: false
	});

	let editorContainer: HTMLDivElement | null = null;
	let monacoEditor: Monaco.editor.IStandaloneCodeEditor | null = null;
	let monacoApi: typeof Monaco | null = null;

	const filteredProjects = $derived(
		searchQuery.trim()
			? projects.filter((p) => p.name.toLowerCase().includes(searchQuery.toLowerCase()))
			: projects
	);

	onMount(async () => {
		window.addEventListener('host-changed', onHostChanged);
		await Promise.all([loadProjects(), loadHosts()]);
		startStatusPoller();
	});

	onDestroy(() => {
		window.removeEventListener('host-changed', onHostChanged);
		if (statusInterval) clearInterval(statusInterval);
	});

	function onHostChanged() {
		loadProjects();
		loadHosts();
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

	async function loadProjects() {
		loading = true;
		try {
			const data = await dockerApi.get<{ projects: ComposeProject[] }>('/docker/compose');
			projects = data.projects || [];
		} catch (e) { console.error(e); } finally { loading = false; }
	}

	// Real-time status poller
	let statusInterval: ReturnType<typeof setInterval> | null = null;
	const pendingUpdates = new Map<string, number>(); // id -> timestamp of last optimistic update

	function startStatusPoller() {
		if (statusInterval) clearInterval(statusInterval);
		console.log('Status poller started, interval: 3s');
		statusInterval = setInterval(async () => {
			try {
				const data = await dockerApi.get<{ projects: ComposeProject[] }>('/docker/compose');
				if (data.projects) {
					// Update each project's status in-place for reactivity
					const now = Date.now();
					for (const newP of data.projects) {
						const idx = projects.findIndex((p) => p.id === newP.id);
						if (idx !== -1) {
							// Skip if this project was recently updated optimistically (within 5s)
							const pending = pendingUpdates.get(newP.id);
							if (pending && now - pending < 10000) continue;
							if (projects[idx].status !== newP.status || projects[idx].running !== newP.running) {
								projects[idx] = { ...projects[idx], status: newP.status, running: newP.running, services: newP.services };
							}
						}
					}
				}
			} catch (e) { console.error('Poller error:', e); }
		}, 3000);
	}

	function showConfirm(title: string, message: string, onConfirm: () => void) {
		confirmDialog = { open: true, title, message, onConfirm };
	}
	function closeConfirm() { confirmDialog.open = false; }

	// --- Actions ---
	let composeSSE: EventSource | null = null;

	function connectComposeSSE(id: string) {
		if (composeSSE) { composeSSE.close(); }
		const token = typeof localStorage !== 'undefined' ? localStorage.getItem('accessToken') || '' : '';
		const hostId = typeof localStorage !== 'undefined' ? localStorage.getItem('currentHostId') || '' : '';
		composeSSE = new EventSource(`/api/v1/docker/compose/${id}/stream?token=${encodeURIComponent(token)}&hostId=${encodeURIComponent(hostId)}`);
		composeSSE.addEventListener('log', ((e: MessageEvent) => {
			try {
				const data = JSON.parse(e.data);
				deployLog.content += data.line + '\n';
				// Auto-scroll
				const el = document.querySelector('[data-deploy-log]');
				if (el) el.scrollTop = el.scrollHeight;
			} catch (e) { console.error('Poller error:', e); }
		}) as EventListener);
		composeSSE.addEventListener('done', (async (e: MessageEvent) => {
			try {
				const data = JSON.parse(e.data);
				deployLog.content += `\n[${data.status}] 操作完成`;
			} catch {
				deployLog.content += '\n[done] 操作完成';
			}
			deployLog.loading = false;
			composeSSE?.close();
			composeSSE = null;
			await loadProjects();
		}) as EventListener);
		composeSSE.onerror = () => {
			deployLog.loading = false;
			composeSSE?.close();
			composeSSE = null;
		};
	}

	function composeUp(id: string, name: string) {
		// 后端根据实际容器状态决定执行 start / up -d / recreate，返回 action
		dockerApi.post<{ action: string }>(`/docker/compose/${id}/up`).then((res) => {
			if (res.action === 'start') {
				// 已有容器只是停止了 → 静默启动
				const idx = projects.findIndex((p) => p.id === id);
				if (idx !== -1) projects[idx] = { ...projects[idx], status: 'running' };
				pendingUpdates.set(id, Date.now());
			} else {
				// 需要 pull/create/recreate → 弹日志窗
				deployLog = { open: true, loading: true, content: '', name, projectId: id };
				connectComposeSSE(id);
			}
		}).catch((e) => {
			deployLog.content = '启动失败: ' + (e instanceof Error ? e.message : String(e));
			deployLog.loading = false;
		});
	}
	function composeDown(id: string, name: string) {
		showConfirm('停止项目', `确定要停止 "${name}" 吗？`, () => {
			const idx = projects.findIndex((p) => p.id === id);
			if (idx !== -1) projects[idx] = { ...projects[idx], status: 'stopped', running: 0 };
			pendingUpdates.set(id, Date.now());
			dockerApi.post(`/docker/compose/${id}/down`).catch(() => {});
		});
	}
	function composeRestart(id: string, name: string) {
		showConfirm('重启项目', `确定要重启 "${name}" 吗？`, () => {
			dockerApi.post(`/docker/compose/${id}/restart`).catch(() => {});
		});
	}
	function composeClean(id: string, name: string) {
		showConfirm('清理项目', `确定要清理 "${name}" 吗？将停止并删除容器和数据卷。`, () => {
			const idx = projects.findIndex((p) => p.id === id);
			if (idx !== -1) projects[idx] = { ...projects[idx], status: 'stopped', running: 0 };
			pendingUpdates.set(id, Date.now());
			dockerApi.post(`/docker/compose/${id}/clean`).catch(() => {});
		});
	}

	function composeRedeploy(id: string, name: string) {
		showConfirm('重新部署', '确定要重新部署这个项目吗？', async () => {
			deployLog = { open: true, loading: true, content: '部署中...\n', name };
			try {
				const result = await dockerApi.post<{ output?: string }>(`/docker/compose/${id}/redeploy`);
				deployLog.content += result?.output || '部署完成';
				await loadProjects();
			} catch (e) {
				deployLog.content += '部署失败: ' + (e instanceof Error ? e.message : String(e));
			} finally { deployLog.loading = false; }
		});
	}
	function viewDeployLog(id: string, name: string) {
		deployLog = { open: true, loading: true, content: '', name };
		dockerApi.get<{ lines?: string[] }>(`/docker/compose/${id}/logs`).then((data) => {
			deployLog.content = (data?.lines && data.lines.length > 0) ? data.lines.join('\n') : '无日志';
		}).catch(() => { deployLog.content = '获取日志失败'; }).finally(() => { deployLog.loading = false; });
	}
	function deleteProject(id: string, name: string) {
		showConfirm('删除项目', `确定要删除 "${name}" 吗？将停止并删除容器。`, () => {
			// Optimistic: remove row immediately
			projects = projects.filter((p) => p.id !== id);
			dockerApi.delete(`/docker/compose/${id}`).catch(() => {});
		});
	}

	// --- Import ---
	async function scanAvailable() {
		importModal = { open: true, loading: true, projects: [], selected: new Set(), importing: false };
		try {
			const data = await dockerApi.get<{ projects: ComposeProject[] }>('/docker/compose/available');
			importModal.projects = data.projects || [];
		} catch (e) { console.error(e); }
		importModal.loading = false;
	}

	function toggleImportProject(name: string) {
		const newSelected = new Set(importModal.selected);
		if (newSelected.has(name)) {
			newSelected.delete(name);
		} else {
			newSelected.add(name);
		}
		importModal.selected = newSelected;
	}

	function selectAllImport() {
		if (importModal.selected.size === importModal.projects.length) {
			importModal.selected = new Set();
		} else {
			importModal.selected = new Set(importModal.projects.map(p => p.name));
		}
	}

	async function doImport() {
		if (importModal.selected.size === 0) return;
		importModal.importing = true;
		try {
			await dockerApi.post('/docker/compose/import', { names: Array.from(importModal.selected) });
			importModal.open = false;
			await loadProjects();
		} catch (e) { console.error(e); }
		importModal.importing = false;
	}

	function abortCompose(id: string) {
		dockerApi.post(`/docker/compose/${id}/abort`).catch(() => {});
		deployLog.content += '\n[aborted] 用户终止操作';
		deployLog.loading = false;
		composeSSE?.close();
		composeSSE = null;
	}

	function closeDeployLog() {
		deployLog.open = false;
		composeSSE?.close();
		composeSSE = null;
	}

	// Check project name on blur
	let nameCheckTimer: ReturnType<typeof setTimeout> | null = null;
	async function checkProjectName(name: string) {
		if (!name.trim()) return;
		try {
			const result = await dockerApi.get<{ exists: boolean }>(`/docker/compose/check-name?name=${encodeURIComponent(name.trim())}`);
			if (result.exists) {
				editorModal.error = '项目名称已存在';
			}
		} catch (e) { /* ignore */ }
	}

	// --- Editor Modal ---
	function openNew() {
		editorModal = {
			open: true, mode: 'new', projectId: '', projectName: '', projectPath: currentHost?.mountPoints?.docker?.path || '/opt/docker',
			composeContent: "services:\n  my-service:\n    image: nginx:latest\n    ports:\n      - \"8080:80\"\n",
			saving: false, error: '', dirty: false, loading: false
		};
		initEditor();
	}

	async function openEdit(project: ComposeProject) {
		editorModal = {
			open: true, mode: 'edit', projectId: project.id, projectName: project.name, projectPath: project.path,
			composeContent: '', saving: false, error: '', dirty: false, loading: true
		};
		try {
			const data = await dockerApi.get<{ content: string }>(`/docker/compose/${project.id}/file`);
			editorModal.composeContent = data?.content || '';
		} catch (e) {
			editorModal.error = e instanceof Error ? e.message : '加载失败';
		} finally {
			editorModal.loading = false;
			initEditor();
		}
	}

	function closeEditor() {
		if (monacoEditor) { monacoEditor.dispose(); monacoEditor = null; }
		editorModal.open = false;
	}

	async function initEditor() {
		if (typeof window === 'undefined') return;
		if (!monacoApi) {
			try {
				const m = await import('monaco-editor');
				monacoApi = m.default || m;
			} catch (e) { console.error(e); return; }
		}
		// Wait for DOM to render the modal
		await new Promise(r => setTimeout(r, 100));
		if (!editorContainer || !monacoApi) return;
		if (monacoEditor) { monacoEditor.dispose(); monacoEditor = null; }

		monacoApi.editor.defineTheme('boxbox-dark', {
			base: 'vs-dark', inherit: true, rules: [],
			colors: { 'editor.background': '#1e1e1e', 'editor.foreground': '#d4d4d4', 'editorLineNumber.foreground': '#5a5a5a', 'editorLineNumber.activeForeground': '#c6c6c6', 'editor.selectionBackground': '#264f78', 'editor.lineHighlightBackground': '#2a2a2a' }
		});

		monacoEditor = monacoApi.editor.create(editorContainer, {
			value: editorModal.composeContent,
			language: 'yaml',
			theme: 'boxbox-dark',
			minimap: { enabled: false },
			fontSize: 13,
			lineNumbers: 'on',
			scrollBeyondLastLine: false,
			automaticLayout: true,
			tabSize: 2,
			wordWrap: 'on'
		});

		monacoEditor.onDidChangeModelContent(() => {
			editorModal.composeContent = monacoEditor?.getValue() || '';
			editorModal.dirty = true;
		});
	}

	async function saveEditor() {
		editorModal.saving = true; editorModal.error = '';
		try {
			if (editorModal.mode === 'new') {
				if (!editorModal.projectName.trim()) { editorModal.error = '请输入项目名称'; editorModal.saving = false; return; }
				await dockerApi.post('/docker/compose', { name: editorModal.projectName.trim(), composeContent: editorModal.composeContent, basePath: editorModal.projectPath });
			} else {
				await dockerApi.put(`/docker/compose/${editorModal.projectId}/file`, { content: editorModal.composeContent });
			}
			closeEditor();
			await loadProjects();
		} catch (e) {
			editorModal.error = e instanceof Error ? e.message : '保存失败';
		} finally { editorModal.saving = false; }
	}

	function getStatusColor(s: string) { return s === 'running' ? 'bg-green-500' : s === 'stopped' ? 'bg-red-500' : s === 'partial' ? 'bg-yellow-500' : 'bg-gray-500'; }
	function getStatusText(s: string) { return s === 'running' ? '运行中' : s === 'stopped' ? '已停止' : s === 'partial' ? '部分运行' : s; }

	const thClass = 'px-3 py-1.5 text-left text-[11px] font-medium uppercase tracking-wider text-text-muted border-b border-border-secondary select-none whitespace-nowrap';
	const tdClass = 'px-3 py-2 text-[13px] text-text-primary border-b border-border-secondary/50';
</script>

<div class="flex h-full flex-col bg-surface-primary">
	<div class="flex items-center justify-between border-b border-border-secondary px-4 py-3">
		<h1 class="text-base font-semibold text-text-primary">
			Compose
			{#if currentHost}<Badge variant="info">{currentHost.name}</Badge>{/if}
			<Badge>{filteredProjects.length}</Badge>
		</h1>
		<div class="flex items-center gap-2">
			<div class="relative">
				<Search size={14} class="absolute left-2.5 top-1/2 -translate-y-1/2 text-text-muted" />
				<input type="text" bind:value={searchQuery} placeholder="搜索..." class="h-7 w-40 rounded border border-border-secondary bg-surface-secondary pl-8 pr-2 text-xs text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none" />
			</div>
			<Button variant="secondary" size="sm" onclick={openNew} title="新建"><Plus size={14} /></Button>
			<Button variant="secondary" size="sm" onclick={scanAvailable} title="扫描导入"><svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24"><g fill="none" stroke="currentColor" stroke-linecap="square" stroke-width="2"><path d="M13 20h9V6H11L9 3.5H2v8.25"/><path d="m6.042 21.502l3.46-3.5l-3.46-3.5m2.258 3.5H.998"/></g></svg></Button>
			<Button variant="secondary" size="sm" onclick={loadProjects} title="刷新"><RefreshCw size={14} /></Button>
		</div>
	</div>
	<div class="flex-1 overflow-auto">
		{#if loading}
			<div class="flex items-center justify-center py-12"><Spinner size="lg" /></div>
		{:else if filteredProjects.length === 0}
			<div class="flex flex-col items-center gap-2 py-12 text-text-muted"><Package size={36} class="opacity-50" /><span class="text-sm">{searchQuery ? '没有匹配的项目' : '暂无 Compose 项目'}</span></div>
		{:else}
			<table class="w-full min-w-[800px] border-collapse text-[13px] leading-5">
				<colgroup><col /><col class="w-[91px]" /><col /><col class="w-[70px]" /><col class="w-[160px]" /></colgroup>
				<thead><tr>
					<th class="{thClass}">Name</th><th class="{thClass}">State</th><th class="{thClass}">Path</th><th class="{thClass}">Services</th><th class="{thClass} text-right">Actions</th>
				</tr></thead>
				<tbody>
					{#each filteredProjects as project (project.id)}
						<tr class="transition-colors hover:bg-surface-secondary">
							<td class="{tdClass}">
								<button type="button" class="flex items-center gap-2 text-left hover:underline" onclick={() => openEdit(project)}>
									<span class="h-2 w-2 shrink-0 rounded-full {getStatusColor(project.status)}"></span>
									<span class="font-medium">{project.name}</span>
								</button>
							</td>
							<td class="{tdClass}"><span class="inline-flex items-center rounded px-1.5 py-0.5 text-[11px] font-medium {project.status === 'running' ? 'bg-green-500/15 text-green-500' : project.status === 'stopped' ? 'bg-red-500/15 text-red-500' : 'bg-gray-500/15 text-gray-400'}">{getStatusText(project.status)}</span></td>
							<td class="{tdClass} text-text-secondary text-[12px]"><span class="block truncate" title={project.path}>{project.path}</span></td>
							<td class="{tdClass} text-text-secondary tabular-nums">{project.running}/{project.services}</td>
							<td class="{tdClass}">
								<div class="flex justify-end gap-1">
									{#if project.status === 'running'}
										<!-- 运行中: 停止 重启 日志 删除 -->
										<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-red-400 transition-colors hover:bg-red-500/10" onclick={() => composeDown(project.id, project.name)} title="停止"><svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="18" height="18" rx="2"/></svg></button>
										<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary" onclick={() => composeRestart(project.id, project.name)} title="重启"><RotateCcw size={13} /></button>
										<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary" onclick={() => viewDeployLog(project.id, project.name)} title="日志"><Eye size={13} /></button>
										<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-red-400 transition-colors hover:bg-red-500/10" onclick={() => deleteProject(project.id, project.name)} title="删除"><Trash2 size={13} /></button>
									{:else if project.status === 'stopped'}
										<!-- 已停止: 启动 清理 日志 删除 -->
										<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-green-500 transition-colors hover:bg-green-500/10" onclick={() => composeUp(project.id, project.name)} title="启动"><Play size={13} /></button>
										<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-orange-400 transition-colors hover:bg-orange-500/10" onclick={() => composeClean(project.id, project.name)} title="清理"><BrushCleaning size={13} /></button>
										<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary" onclick={() => viewDeployLog(project.id, project.name)} title="日志"><Eye size={13} /></button>
										<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-red-400 transition-colors hover:bg-red-500/10" onclick={() => deleteProject(project.id, project.name)} title="删除"><Trash2 size={13} /></button>
									{:else}
										<!-- 未构建/异常: 启动 清理 日志 -->
										<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-green-500 transition-colors hover:bg-green-500/10" onclick={() => composeUp(project.id, project.name)} title="启动"><Play size={13} /></button>
										<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-orange-400 transition-colors hover:bg-orange-500/10" onclick={() => composeClean(project.id, project.name)} title="清理"><BrushCleaning size={13} /></button>
										<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary" onclick={() => viewDeployLog(project.id, project.name)} title="日志"><Eye size={13} /></button>
									{/if}
								</div>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</div>
</div>

<!-- Import Modal -->
{#if importModal.open}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
		<div class="flex h-[60vh] w-[600px] flex-col rounded-lg bg-surface-primary shadow-xl border border-border-secondary">
			<div class="flex items-center justify-between border-b border-border-secondary px-4 py-3">
				<h3 class="text-sm font-semibold text-text-primary">扫描导入 Compose 项目</h3>
				<button type="button" class="text-text-muted hover:text-text-primary" onclick={() => { importModal.open = false; }}><X size={16} /></button>
			</div>
			<div class="flex-1 overflow-auto p-4">
				{#if importModal.loading}
					<div class="flex items-center justify-center py-8"><Spinner /></div>
				{:else if importModal.projects.length === 0}
					<p class="text-sm text-text-secondary text-center py-8">没有发现可导入的项目</p>
				{:else}
					<div class="mb-3 flex items-center gap-2">
						<button type="button" class="text-xs text-accent hover:underline" onclick={selectAllImport}>
							{importModal.selected.size === importModal.projects.length ? '取消全选' : '全选'}
						</button>
						<span class="text-xs text-text-muted">已选 {importModal.selected.size}/{importModal.projects.length}</span>
					</div>
					<div class="flex flex-col gap-2">
						{#each importModal.projects as project}
							<label class="flex items-center gap-3 rounded border border-border-secondary bg-surface-secondary px-3 py-2 cursor-pointer hover:border-border-focus transition-colors">
								<input type="checkbox" checked={importModal.selected.has(project.name)} onchange={() => toggleImportProject(project.name)} class="rounded" />
								<div class="flex-1">
									<div class="text-sm font-medium text-text-primary">{project.name}</div>
									<div class="text-xs text-text-muted">{project.path}</div>
								</div>
							</label>
						{/each}
					</div>
				{/if}
			</div>
			<div class="flex justify-end gap-2 border-t border-border-secondary px-4 py-3">
				<Button variant="secondary" size="sm" onclick={() => { importModal.open = false; }}>取消</Button>
				<Button variant="primary" size="sm" onclick={doImport} disabled={importModal.selected.size === 0 || importModal.importing}>
					{#if importModal.importing}<Spinner size={14} class="mr-1" /> 导入中...{:else}导入选中项目{/if}
				</Button>
			</div>
		</div>
	</div>
{/if}

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

<LogModal
	open={deployLog.open}
	name="部署日志 - {deployLog.name}"
	content={deployLog.content}
	loading={deployLog.loading}
	streaming={deployLog.loading}
	onClose={closeDeployLog}
/>

<!-- Editor Modal (New + Edit) -->
{#if editorModal.open}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
		<div class="flex h-[85vh] w-[900px] flex-col rounded-lg bg-surface-primary shadow-xl border border-border-secondary">
			<div class="flex items-center justify-between border-b border-border-secondary px-4 py-3">
				<div class="flex items-center gap-3">
					<h3 class="text-sm font-semibold text-text-primary">{editorModal.mode === 'new' ? '新建 Compose 项目' : editorModal.projectName}</h3>
					{#if editorModal.dirty}<span class="text-[11px] text-orange-400">● 已修改</span>{/if}
					{#if editorModal.error}<span class="text-xs text-red-400">{editorModal.error}</span>{/if}
				</div>
				<div class="flex items-center gap-2">
					{#if editorModal.mode === 'new'}
						<span class="flex h-7 items-center rounded border border-border-secondary bg-surface-tertiary px-2 text-xs text-text-muted"><span class="whitespace-nowrap">{(currentHost?.mountPoints?.docker?.path || "/opt/docker")}/</span><input type="text" bind:value={editorModal.projectName} placeholder="{name}" class="w-36 border-none bg-transparent px-0 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:ring-0" onblur={() => checkProjectName(editorModal.projectName)} oninput={() => { editorModal.error = ''; }} /></span>
					{/if}
					<Button variant="secondary" size="sm" onclick={closeEditor}>取消</Button>
					<Button variant="primary" size="sm" onclick={saveEditor} disabled={editorModal.saving || (editorModal.mode === 'edit' && !editorModal.dirty)}>
						{#if editorModal.saving}<Spinner size={14} class="mr-1" /> 保存中...{:else}<Save size={14} class="mr-1" /> {editorModal.mode === 'new' ? '创建' : '保存'}{/if}
					</Button>
				</div>
			</div>
			<div class="flex-1 overflow-hidden">
				{#if editorModal.loading}<div class="flex items-center justify-center py-12"><Spinner size="lg" /></div>
				{:else}<div bind:this={editorContainer} class="h-full w-full"></div>{/if}
			</div>
		</div>
	</div>
{/if}
