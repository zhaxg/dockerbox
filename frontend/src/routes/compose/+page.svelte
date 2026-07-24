<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { Spinner, Button, Badge } from '$lib/components/ui';
	import { hostsApi, type DockerHostsConfig } from '$lib/api/hosts';
	import { api } from '$lib/api/client';
	import { Play, RefreshCw, Eye, RotateCcw, Hammer, Package, Plus, Search, Trash2, X, Save } from 'lucide-svelte';
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
	let deployLog = $state<{ open: boolean; loading: boolean; content: string; name: string }>({
		open: false, loading: false, content: '', name: ''
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
		open: false, mode: 'new', projectId: '', projectName: '', projectPath: '/vol1/1000/docker',
		composeContent: '', saving: false, error: '', dirty: false, loading: false
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
	});

	onDestroy(() => {
		window.removeEventListener('host-changed', onHostChanged);
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
			const data = await api.get<{ projects: ComposeProject[] }>('/docker/compose');
			projects = data.projects || [];
		} catch (e) { console.error(e); } finally { loading = false; }
	}

	function showConfirm(title: string, message: string, onConfirm: () => void) {
		confirmDialog = { open: true, title, message, onConfirm };
	}
	function closeConfirm() { confirmDialog.open = false; }

	// --- Actions ---
	async function composeUp(id: string) {
		try { await api.post(`/docker/compose/${id}/up`); await loadProjects(); } catch (e) { console.error(e); }
	}
	function composeDown(id: string) {
		showConfirm('停止 Compose', '确定要停止这个项目吗？', async () => {
			try { await api.post(`/docker/compose/${id}/down`); await loadProjects(); } catch (e) { console.error(e); }
		});
	}
	function composeRestart(id: string) {
		showConfirm('重启 Compose', '确定要重启这个项目吗？', async () => {
			try { await api.post(`/docker/compose/${id}/restart`); await loadProjects(); } catch (e) { console.error(e); }
		});
	}
	function composeRedeploy(id: string, name: string) {
		showConfirm('重新部署', '确定要重新部署这个项目吗？', async () => {
			deployLog = { open: true, loading: true, content: '部署中...\n', name };
			try {
				const result = await api.post<{ output?: string }>(`/docker/compose/${id}/redeploy`);
				deployLog.content += result?.output || '部署完成';
				await loadProjects();
			} catch (e) {
				deployLog.content += '部署失败: ' + (e instanceof Error ? e.message : String(e));
			} finally { deployLog.loading = false; }
		});
	}
	function viewDeployLog(id: string, name: string) {
		deployLog = { open: true, loading: true, content: '', name };
		api.get<{ lines?: string[] }>(`/docker/compose/${id}/logs`).then((data) => {
			deployLog.content = (data?.lines && data.lines.length > 0) ? data.lines.join('\n') : '无日志';
		}).catch(() => { deployLog.content = '获取日志失败'; }).finally(() => { deployLog.loading = false; });
	}
	function deleteProject(id: string, name: string) {
		showConfirm('删除项目', `确定要删除 "${name}" 项目及其文件吗？此操作不可恢复。`, async () => {
			try { await api.delete(`/docker/compose/${id}`); await loadProjects(); } catch (e) { console.error(e); }
		});
	}

	// --- Editor Modal ---
	function openNew() {
		editorModal = {
			open: true, mode: 'new', projectId: '', projectName: '', projectPath: '/vol1/1000/docker',
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
			const data = await api.get<{ content: string }>(`/docker/compose/${project.id}/file`);
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
				await api.post('/docker/compose', { name: editorModal.projectName.trim(), composeContent: editorModal.composeContent, basePath: editorModal.projectPath });
			} else {
				await api.put(`/docker/compose/${editorModal.projectId}/file`, { content: editorModal.composeContent });
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
				<colgroup><col /><col class="w-[70px]" /><col /><col class="w-[70px]" /><col class="w-[140px]" /></colgroup>
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
										<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-red-400 transition-colors hover:bg-red-500/10" onclick={() => composeDown(project.id)} title="停止"><svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="18" height="18" rx="2"/></svg></button>
										<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary" onclick={() => composeRestart(project.id)} title="重启"><RotateCcw size={13} /></button>
										<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary" onclick={() => composeRedeploy(project.id, project.name)} title="重新部署"><Hammer size={13} /></button>
										<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary" onclick={() => viewDeployLog(project.id, project.name)} title="部署日志"><Eye size={13} /></button>
									{:else}
										<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-green-500 transition-colors hover:bg-green-500/10" onclick={() => composeUp(project.id)} title="启动"><Play size={13} /></button>
										<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary" onclick={() => viewDeployLog(project.id, project.name)} title="部署日志"><Eye size={13} /></button>
									{/if}
									<button
										type="button"
										class="inline-flex h-6 w-6 items-center justify-center rounded transition-colors {project.status === 'running' ? 'cursor-not-allowed text-text-muted/40' : 'text-red-400 hover:bg-red-500/10'}"
										onclick={() => { if (project.status !== 'running') deleteProject(project.id, project.name); }}
										title={project.status === 'running' ? '运行中不可删除' : '删除'}
									><Trash2 size={13} /></button>
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

<!-- Deploy Log Modal -->
{#if deployLog.open}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
		<div class="flex h-[70vh] w-[700px] flex-col rounded-lg bg-surface-primary shadow-xl border border-border-secondary">
			<div class="flex items-center justify-between border-b border-border-secondary px-4 py-3">
				<h3 class="text-sm font-semibold text-text-primary">部署日志 - {deployLog.name}</h3>
				<button type="button" class="text-text-muted hover:text-text-primary" onclick={() => { deployLog.open = false; }}><X size={16} /></button>
			</div>
			<div class="flex-1 overflow-auto p-4">
				{#if deployLog.loading}<div class="flex items-center justify-center py-8"><Spinner /></div>
				{:else}<pre class="whitespace-pre-wrap font-mono text-xs text-text-secondary">{deployLog.content}</pre>{/if}
			</div>
		</div>
	</div>
{/if}

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
						<input type="text" bind:value={editorModal.projectName} placeholder="项目名称" class="h-7 w-36 rounded border border-border-secondary bg-surface-secondary px-2 text-xs text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none" />
						<input type="text" bind:value={editorModal.projectPath} placeholder="存储路径" class="h-7 w-44 rounded border border-border-secondary bg-surface-secondary px-2 text-xs text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none" />
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
