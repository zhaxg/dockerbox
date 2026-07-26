<script lang="ts">
	import { onMount } from 'svelte';
	import { Spinner, Button, Badge } from '$lib/components/ui';
	import { hostsApi, type DockerHost, type DockerHostsConfig } from '$lib/api/hosts';
	import { Plus, RefreshCw, Trash2, Pencil, Plug, X, Copy, Check, Key, Terminal, MessageSquare, Search, FileUp } from 'lucide-svelte';

	let hostsConfig = $state<DockerHostsConfig>({ default: '', hosts: {} });
	let loading = $state(true);
	let searchQuery = $state('');
	let confirmDialog = $state<{ open: boolean; title: string; message: string; onConfirm: () => void }>({
		open: false, title: '', message: '', onConfirm: () => {}
	});
	let testLoading = $state(false);
	let copied = $state<Record<string, boolean>>({});
	let genKeyLoading = $state(false);
	let hostStats = $state<Record<string, { status: string; total: number; running: number; stopped: number }>>({});
	let toastMsg = $state('');
	let toastType = $state<'ok' | 'err'>('ok');

	let modal = $state<{
		open: boolean; mode: 'add' | 'edit'; host: any;
		mountKey: string; mountPath: string; mountReadOnly: boolean; dockerDirKey: string;
		isDefault: boolean;
	}>({
		open: false, mode: 'add',
		host: { id: '', name: '', driver: 'socket', endpoint: '/var/run/docker.sock', sshKey: '', sshPubKey: '', tags: [], mountPoints: {}, key: '' },
		mountKey: '', mountPath: '', mountReadOnly: false, dockerDirKey: '', isDefault: false
	});

	const hostList = $derived(Object.entries(hostsConfig.hosts || {}).map(([key, host]) => ({key, ...host})));
	const filteredHostList = $derived(
		searchQuery.trim()
			? hostList.filter((h) => h.name.toLowerCase().includes(searchQuery.toLowerCase()) || h.endpoint.toLowerCase().includes(searchQuery.toLowerCase()))
			: hostList
	);
	onMount(loadHosts);

	async function loadHosts() {
		loading = true;
		try {
			hostsConfig = await hostsApi.list();
			if (!hostsConfig.hosts) hostsConfig.hosts = {};
			for (const id of Object.keys(hostsConfig.hosts)) fetchHostStats(id);
		} catch (e) { console.error(e); }
		finally { loading = false; }
	}

	async function fetchHostStats(id: string) {
		try {
			const token = localStorage.getItem('accessToken') || '';
			const resp = await fetch(`/api/v1/hosts/${id}/stats`, { headers: { 'Authorization': 'Bearer ' + token } });
			if (resp.ok) hostStats[id] = await resp.json();
		} catch (e) { hostStats[id] = { status: 'offline', total: 0, running: 0, stopped: 0 }; }
	}

	function showToast(msg: string, type: 'ok' | 'err' = 'ok') {
		toastMsg = msg; toastType = type;
		setTimeout(() => toastMsg = '', 4000);
	}

	function showConfirm(title: string, message: string, onConfirm: () => void) {
		confirmDialog = { open: true, title, message, onConfirm };
	}
	function closeConfirm() { confirmDialog.open = false; }

	function openAdd() {
		modal = {
			open: true, mode: 'add',
			host: { id: 'host-' + Date.now().toString(36), name: '', driver: 'socket', endpoint: '/var/run/docker.sock', sshKey: '', tags: [], mountPoints: { docker: { path: '/var/docker', readOnly: false } } },
			mountKey: '', mountPath: '', mountReadOnly: false, dockerDirKey: 'docker', isDefault: false
		};
	}

	function openEdit(host: any) {
		const mp = host.mountPoints || {};
		// Ensure docker key exists
		if (!mp.docker) mp.docker = { path: '', readOnly: false };
		modal = {
			open: true, mode: 'edit',
			host: { ...host, id: host.key, mountPoints: { ...mp }, tags: [...(host.tags || [])] },
			mountKey: '', mountPath: '', mountReadOnly: false, dockerDirKey: 'docker',
			isDefault: hostsConfig.default === host.key
		};
	}

	function closeModal() { modal.open = false; }

	function addMountPoint() {
		const key = modal.mountKey.trim(), path = modal.mountPath.trim();
		if (!key || !path) return;
		if (!modal.host.mountPoints) modal.host.mountPoints = {};
		modal.host.mountPoints[key] = { path, readOnly: modal.mountReadOnly };
		modal.mountKey = ''; modal.mountPath = ''; modal.mountReadOnly = false;
	}

	function removeMountPoint(key: string) {
		if (modal.host.mountPoints && key !== 'docker') {
			delete modal.host.mountPoints[key];
			modal.host.mountPoints = { ...modal.host.mountPoints };
		}
	}

	async function genKeyPair() {
		genKeyLoading = true;
		try {
			const result = await fetch('/api/v1/ssh/genkey', {
				method: 'POST', headers: { 'Authorization': 'Bearer ' + (localStorage.getItem('accessToken') || '') }
			}).then(r => r.json());
			if (result.private_key) { modal.host.sshKey = result.private_key; modal.host.sshPubKey = result.public_key; }
		} catch (e) { console.error(e); }
		finally { genKeyLoading = false; }
	}

	let keyFileInput: HTMLInputElement;
	function handleKeyFileUpload(e: Event) {
		const file = (e.target as HTMLInputElement).files?.[0];
		if (!file) return;
		const reader = new FileReader();
		reader.onload = () => {
			const text = reader.result as string;
			modal.host.sshKey = text;
			// Try extract public key comment to guess endpoint user
		};
		reader.readAsText(file);
		// Reset so same file can be re-selected
		(e.target as HTMLInputElement).value = '';
	}

	function copyText(text: string, key: string) {
		if (navigator.clipboard && window.isSecureContext) {
			navigator.clipboard.writeText(text);
		} else {
			const ta = document.createElement('textarea');
			ta.value = text; ta.style.position = 'fixed'; ta.style.left = '-9999px';
			document.body.appendChild(ta); ta.select(); document.execCommand('copy');
			document.body.removeChild(ta);
		}
		copied[key] = true;
		setTimeout(() => copied[key] = false, 2000);
	}

	function getCopyCmd() {
		return `mkdir -p ~/.ssh && chmod 700 ~/.ssh && echo '${modal.host.sshPubKey || ''}' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys`;
	}

	async function doSave() {
		// Validate docker directory path
		const dockerMP = modal.host.mountPoints?.docker;
		if (!dockerMP || !dockerMP.path?.trim()) {
			showToast('Docker主目录路径不能为空', 'err');
			return;
		}
		const saveData = { ...modal.host, isDefault: modal.isDefault };
		try {
			if (modal.mode === 'add') await hostsApi.create(saveData);
			else await hostsApi.update(modal.host.id, saveData);
			hostsConfig.default = modal.isDefault ? modal.host.id : (hostsConfig.default === modal.host.id ? '' : hostsConfig.default);
			await loadHosts();
		} catch (e) { console.error(e); }
	}

	function saveAndClose() { doSave().then(() => closeModal()); }

	async function saveAndTest() {
		await doSave();
		testLoading = true;
		try {
			const result = await hostsApi.test(modal.host.id);
			showToast(result.status === 'ok' ? '连接成功: ' + result.message : '连接失败: ' + result.message, result.status === 'ok' ? 'ok' : 'err');
		} catch (e) { showToast('连接失败: ' + String(e), 'err'); }
		finally { testLoading = false; }
	}

	function deleteHost(id: string, name: string) {
		showConfirm('删除主机', `确定要删除 "${name}" 吗？`, async () => {
			try { await hostsApi.delete(id); await loadHosts(); } catch (e) { console.error(e); }
		});
	}

	async function testHost(id: string) {
		testLoading = true;
		try {
			const result = await hostsApi.test(id);
			showToast(result.status === 'ok' ? '连接成功: ' + result.message : '连接失败: ' + result.message, result.status === 'ok' ? 'ok' : 'err');
		} catch (e) { showToast('连接失败: ' + String(e), 'err'); }
		finally { testLoading = false; }
	}

	function getDriverLabel(d: string) { return d === 'ssh' ? 'SSH' : 'Socket'; }
	function getDriverColor(d: string) { return d === 'ssh' ? 'text-green-400' : 'text-orange-400'; }

	const thClass = 'px-3 py-1.5 text-left text-[11px] font-medium uppercase tracking-wider text-text-muted border-b border-border-secondary select-none whitespace-nowrap';
	const tdClass = 'px-3 py-2 text-[13px] text-text-primary border-b border-border-secondary/50';
</script>

<div class="flex h-full flex-col bg-surface-primary">
	<div class="flex items-center justify-between border-b border-border-secondary px-4 py-3">
		<h1 class="text-base font-semibold text-text-primary">主机 <Badge>{hostList.length}</Badge></h1>
		<div class="flex items-center gap-2">
			<div class="relative">
				<Search size={14} class="absolute left-2.5 top-1/2 -translate-y-1/2 text-text-muted" />
				<input type="text" bind:value={searchQuery} placeholder="搜索主机..."
					class="h-7 w-48 rounded border border-border-secondary bg-surface-secondary pl-8 pr-2 text-xs text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none" />
			</div>
			<Button variant="secondary" size="sm" onclick={openAdd} title="添加主机"><Plus size={14} /></Button>
			<Button variant="secondary" size="sm" onclick={loadHosts} title="刷新"><RefreshCw size={14} /></Button>
		</div>
	</div>
	<div class="flex-1 overflow-auto">
		{#if loading}
			<div class="flex items-center justify-center py-12"><Spinner size="lg" /></div>
		{:else if hostList.length === 0}
			<div class="flex flex-col items-center gap-2 py-12 text-text-muted">
				<span class="text-sm">{searchQuery ? "没有匹配的主机" : "暂无主机配置"}</span>
				<Button variant="primary" size="sm" onclick={openAdd}><Plus size={14} class="mr-1" /> 添加主机</Button>
			</div>
		{:else}
			<table class="w-full min-w-[900px] border-collapse text-[13px] leading-5">
				<thead><tr>
					<th class="{thClass}">Name</th>
					<th class="{thClass}">Endpoint</th>
					<th class="{thClass}">Status</th>
					<th class="{thClass}">Tags</th>
					<th class="{thClass}">Docker</th>
					<th class="{thClass} text-right">Actions</th>
				</tr></thead>
				<tbody>
					{#each filteredHostList as host (host.key)}
						<tr class="transition-colors hover:bg-surface-secondary">
							<td class="{tdClass}">
								<div class="flex items-center gap-2">
									{#if hostStats[host.key]}
										<span class="h-2 w-2 rounded-full shrink-0 {hostStats[host.key].status === 'online' ? 'bg-green-500' : 'bg-red-500'}"></span>
									{:else}<span class="h-2 w-2 rounded-full bg-gray-500 shrink-0"></span>{/if}
									<span class="font-medium">{host.name}</span>
									<span class="text-[11px] {getDriverColor(host.driver)}">{getDriverLabel(host.driver)}</span>
								</div>
							</td>
							<td class="{tdClass} font-mono text-[12px] text-text-secondary">{host.endpoint}</td>
							<td class="{tdClass}">
								{#if hostStats[host.key]}
									<span class="text-[11px] {hostStats[host.key].status === 'online' ? 'text-green-400' : 'text-red-400'}">
										{hostStats[host.key].status === 'online' ? '在线' : '离线'}
									</span>
								{:else}<span class="text-[11px] text-text-muted">检测中...</span>{/if}
							</td>
							<td class="{tdClass}">
								<div class="flex flex-wrap gap-1">
									{#each (host.tags || []) as tag}
										<span class="inline-flex items-center rounded bg-surface-tertiary px-1.5 py-0.5 text-[10px] text-text-secondary">{tag}</span>
									{/each}
								</div>
							</td>
							<td class="{tdClass} text-text-secondary text-[12px]">
								{#if hostStats[host.key]}{hostStats[host.key].running}/{hostStats[host.key].total}{:else}-{/if}
							</td>
							<td class="{tdClass}">
								<div class="flex justify-end items-center gap-1">
									{#if hostsConfig.default === host.key}
										<Check size={14} class="text-green-400 shrink-0" />
									{/if}
									<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary" onclick={() => testHost(host.key)} title="测试连接" disabled={testLoading}><Plug size={13} /></button>
									<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary" onclick={() => openEdit(host)} title="编辑"><Pencil size={13} /></button>
									<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-red-400 transition-colors hover:bg-red-500/10" onclick={() => deleteHost(host.key, host.name)} title="删除"><Trash2 size={13} /></button>
								</div>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</div>
</div>

<!-- Toast -->
{#if toastMsg}
	<div class="fixed bottom-4 right-4 z-[70] max-w-md rounded-lg border px-4 py-3 shadow-xl {toastType === 'ok' ? 'border-green-500/30 bg-green-500/10 text-green-400' : 'border-red-500/30 bg-red-500/10 text-red-400'}">
		<div class="flex items-center gap-2 text-[12px]"><MessageSquare size={14} /><span>{toastMsg}</span></div>
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

<!-- Add/Edit Modal -->
{#if modal.open}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
		<div class="flex max-h-[85vh] w-[700px] flex-col rounded-lg bg-surface-primary shadow-xl border border-border-secondary">
			<div class="flex items-center justify-between border-b border-border-secondary px-4 py-3">
				<h3 class="text-sm font-semibold text-text-primary">{modal.mode === 'add' ? '添加主机' : '编辑主机 - ' + modal.host.name}</h3>
				<button type="button" class="text-text-muted hover:text-text-primary" onclick={closeModal}><X size={16} /></button>
			</div>
			<div class="flex-1 overflow-auto p-4 space-y-4">
				<div class="flex items-end gap-3">
					<div class="flex-1">
						<label class="mb-1 block text-[11px] text-text-muted">显示名称 <span class="text-red-400">*</span></label>
						<input type="text" bind:value={modal.host.name} placeholder="主NAS" class="w-full rounded border border-border-secondary bg-surface-secondary px-3 py-1.5 text-xs text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none" />
					</div>
					<label class="flex items-center gap-1.5 text-[11px] text-text-muted cursor-pointer pb-1.5 whitespace-nowrap">
						<input type="checkbox" bind:checked={modal.isDefault} class="rounded accent-green-500" /> 默认主机
					</label>
				</div>
				<div class="grid grid-cols-3 gap-3">
					<div>
						<label class="mb-1 block text-[11px] text-text-muted">连接方式</label>
						<select bind:value={modal.host.driver} onchange={(e) => { if ((e.target as HTMLSelectElement).value === 'socket' && !modal.host.endpoint) modal.host.endpoint = '/var/run/docker.sock'; }}
							class="w-full rounded border border-border-secondary bg-surface-secondary px-3 py-1.5 text-xs text-text-primary focus:border-border-focus focus:outline-none">
							<option value="socket">Socket</option>
							<option value="ssh">SSH</option>
						</select>
					</div>
					<div class="col-span-2">
						<label class="mb-1 block text-[11px] text-text-muted">{modal.host.driver === 'ssh' ? 'user@host:port' : '/var/run/docker.sock'}</label>
						<input type="text" bind:value={modal.host.endpoint}
							placeholder={modal.host.driver === 'ssh' ? 'root@192.168.1.100:22' : '/var/run/docker.sock'}
							class="w-full rounded border border-border-secondary bg-surface-secondary px-3 py-1.5 text-xs text-text-primary font-mono placeholder:text-text-muted focus:border-border-focus focus:outline-none" />
					</div>
				</div>
				{#if modal.host.driver === 'ssh'}
					<div class="rounded border border-border-secondary bg-surface-secondary p-3 space-y-3">
						<div class="flex items-center justify-between">
							<span class="text-[11px] font-medium text-text-muted flex items-center gap-1"><Key size={12} /> ED25519 密钥对</span>
							<div class="flex items-center gap-1.5">
								<input type="file" bind:this={keyFileInput} accept=".pem,.key,id_ed25519,id_rsa,*.pem,*.key" class="hidden" onchange={handleKeyFileUpload} />
								<Button variant="secondary" size="sm" onclick={() => keyFileInput?.click()}>
									<FileUp size={12} class="mr-1" />上传私钥
								</Button>
								<Button variant="secondary" size="sm" onclick={genKeyPair} disabled={genKeyLoading}>
									{#if genKeyLoading}<Spinner size={12} />{:else}一键生成密钥对{/if}
								</Button>
							</div>
						</div>
						<div>
							<label class="mb-1 block text-[10px] text-text-muted">私钥</label>
							<textarea bind:value={modal.host.sshKey} rows={3} placeholder="点击生成或粘贴"
								class="w-full rounded border border-border-secondary bg-black/30 px-2 py-1 text-[11px] text-green-400 font-mono placeholder:text-text-muted focus:border-border-focus focus:outline-none resize-none"></textarea>
						</div>
						<div>
							<label class="mb-1 block text-[10px] text-text-muted">公钥</label>
							<input type="text" bind:value={modal.host.sshPubKey} placeholder="点击生成或粘贴"
								class="w-full rounded border border-border-secondary bg-black/30 px-2 py-1 text-[11px] text-green-400 font-mono placeholder:text-text-muted focus:border-border-focus focus:outline-none" />
						</div>
						<div class="flex items-center gap-1 pt-2 border-t border-border-secondary">
							<button type="button" class="inline-flex items-center gap-1 rounded px-2 py-1 text-[10px] text-text-secondary hover:bg-surface-tertiary hover:text-text-primary transition-colors" onclick={() => copyText(modal.host.sshKey || '', 'priv')} disabled={!modal.host.sshKey}>
								{#if copied['priv']}<Check size={10} class="text-green-400" />已复制{:else}<Copy size={10} />私钥{/if}
							</button>
							<button type="button" class="inline-flex items-center gap-1 rounded px-2 py-1 text-[10px] text-text-secondary hover:bg-surface-tertiary hover:text-text-primary transition-colors" onclick={() => copyText(modal.host.sshPubKey || '', 'pub')} disabled={!modal.host.sshPubKey}>
								{#if copied['pub']}<Check size={10} class="text-green-400" />已复制{:else}<Copy size={10} />公钥{/if}
							</button>
							<button type="button" class="inline-flex items-center gap-1 rounded px-2 py-1 text-[10px] text-text-secondary hover:bg-surface-tertiary hover:text-text-primary transition-colors" onclick={() => copyText(getCopyCmd(), 'cmd')} disabled={!modal.host.sshPubKey || !modal.host.endpoint}>
								{#if copied['cmd']}<Check size={10} class="text-green-400" />已复制{:else}<Terminal size={10} />公钥设置命令{/if}
							</button>
							<button type="button" class="inline-flex items-center gap-1 rounded px-2 py-1 text-[10px] text-text-secondary hover:bg-surface-tertiary hover:text-text-primary transition-colors" onclick={() => copyText('systemctl enable --now podman.socket', 'podman')}>
								{#if copied['podman']}<Check size={10} class="text-green-400" />已复制{:else}<Terminal size={10} />Podman命令{/if}
							</button>
						</div>
					</div>
				{/if}
				<div>
					<label class="mb-1 block text-[11px] text-text-muted">标签（逗号分隔）</label>
					<input type="text" value={(modal.host.tags || []).join(', ')} onchange={(e) => { modal.host.tags = (e.target as HTMLInputElement).value.split(',').map(s => s.trim()).filter(Boolean); }} placeholder="home, nas, prod" class="w-full rounded border border-border-secondary bg-surface-secondary px-3 py-1.5 text-xs text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none" />
				</div>
				<div>
					<label class="mb-2 block text-[11px] text-text-muted font-medium">挂载目录</label>
						<div class="space-y-1 mb-2">
							<!-- Docker 主目录（固定首行，不可删除） -->
							<div class="flex items-center gap-2 px-1 py-1">
								<span class="text-[12px] font-medium text-text-secondary min-w-[64px]">docker</span>
								<input type="text" value={modal.host.mountPoints?.docker?.path || ''} onchange={(e) => { if (!modal.host.mountPoints) modal.host.mountPoints = {}; modal.host.mountPoints.docker = { path: (e.target as HTMLInputElement).value, readOnly: false }; }} placeholder="/var/docker" class="flex-1 rounded border border-border-secondary bg-surface-primary px-2 py-1 text-[11px] text-text-primary font-mono focus:border-border-focus focus:outline-none" />
								<span class="text-[11px] text-green-400 font-medium shrink-0">Docker主目录</span>
							</div>
							<!-- 其他挂载目录 -->
							{#each Object.entries(modal.host.mountPoints || {}) as [key, mp]}
								{#if key !== 'docker'}
									<div class="flex items-center gap-2 px-1 py-1">
										<span class="text-[12px] font-medium text-text-secondary min-w-[64px]">{key}</span>
										<span class="flex-1 text-[11px] text-text-primary font-mono truncate">{mp.path}</span>
										{#if mp.readOnly}<span class="text-[10px] text-text-muted shrink-0">只读</span>{/if}
										<button type="button" class="text-text-muted hover:text-red-400 shrink-0" onclick={() => removeMountPoint(key)}><X size={12} /></button>
								</div>
							{/if}
						{/each}
					</div>
					<div class="flex items-center gap-2">
						<input type="text" bind:value={modal.mountKey} placeholder="名称" class="w-28 rounded border border-border-secondary bg-surface-secondary px-2 py-1 text-[11px] text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none" />
						<input type="text" bind:value={modal.mountPath} placeholder="/opt/docker" class="flex-1 rounded border border-border-secondary bg-surface-secondary px-2 py-1 text-[11px] text-text-primary font-mono placeholder:text-text-muted focus:border-border-focus focus:outline-none" />
						<label class="flex items-center gap-1 text-[11px] text-text-muted cursor-pointer"><input type="checkbox" bind:checked={modal.mountReadOnly} class="rounded" /> 只读</label>
						<button type="button" class="rounded bg-surface-tertiary px-2 py-1 text-[11px] text-text-primary hover:bg-surface-secondary" onclick={addMountPoint}>添加</button>
					</div>
				</div>
			</div>
			<div class="flex items-center justify-between border-t border-border-secondary px-4 py-3">
				<Button variant="ghost" size="sm" onclick={saveAndTest} disabled={testLoading || !modal.host.name || !modal.host.endpoint}>
					{#if testLoading}<Spinner size={12} class="mr-1" />{:else}<Plug size={12} class="mr-1" />{/if}测试连接
				</Button>
				<div class="flex items-center gap-2">
					<Button variant="secondary" size="sm" onclick={closeModal}>取消</Button>
					<Button variant="primary" size="sm" onclick={saveAndClose} disabled={!modal.host.name || !modal.host.endpoint}>
						{modal.mode === 'add' ? '添加' : '保存'}
					</Button>
				</div>
			</div>
		</div>
	</div>
{/if}
