<script lang="ts">
	import { onMount } from 'svelte';
	import { Spinner, Button } from '$lib/components/ui';
	import { hostsApi, type DockerHost, type DockerHostsConfig, type HostMountPoint } from '$lib/api/hosts';
	import { Plus, RefreshCw, Trash2, Pencil, Plug, X, Copy, Check, ChevronDown, ChevronUp } from 'lucide-svelte';

	let hostsConfig = $state<DockerHostsConfig>({ default: '', hosts: {} });
	let loading = $state(true);
	let confirmDialog = $state<{ open: boolean; title: string; message: string; onConfirm: () => void }>({
		open: false, title: '', message: '', onConfirm: () => {}
	});
	let testResult = $state<{ hostId: string; status: string; message: string } | null>(null);
	let testLoading = $state(false);
	let sshCopied = $state<Record<string, boolean>>({});

	// Modal
	let modal = $state<{
		open: boolean;
		mode: 'add' | 'edit';
		host: DockerHost;
		mountKey: string;
		mountPath: string;
		mountReadOnly: boolean;
		showSsh: boolean;
	}>({
		open: false, mode: 'add',
		host: { id: '', name: '', driver: 'ssh', endpoint: '', tags: [], mountPoints: {} },
		mountKey: '', mountPath: '', mountReadOnly: false, showSsh: false
	});

	const hostList = $derived(Object.values(hostsConfig.hosts || {}));

	onMount(loadHosts);

	async function loadHosts() {
		loading = true;
		try {
			hostsConfig = await hostsApi.list();
			if (!hostsConfig.hosts) hostsConfig.hosts = {};
		} catch (e) { console.error(e); }
		finally { loading = false; }
	}

	function showConfirm(title: string, message: string, onConfirm: () => void) {
		confirmDialog = { open: true, title, message, onConfirm };
	}
	function closeConfirm() { confirmDialog.open = false; }

	function openAdd() {
		modal = {
			open: true, mode: 'add',
			host: { id: '', name: '', driver: 'ssh', endpoint: '', tags: [], mountPoints: {} },
			mountKey: '', mountPath: '', mountReadOnly: false, showSsh: false
		};
	}

	function openEdit(host: DockerHost) {
		modal = {
			open: true, mode: 'edit',
			host: { ...host, mountPoints: { ...(host.mountPoints || {}) }, tags: [...(host.tags || [])] },
			mountKey: '', mountPath: '', mountReadOnly: false, showSsh: false
		};
	}

	function closeModal() { modal.open = false; }

	function addMountPoint() {
		const key = modal.mountKey.trim();
		const path = modal.mountPath.trim();
		if (!key || !path) return;
		if (!modal.host.mountPoints) modal.host.mountPoints = {};
		modal.host.mountPoints[key] = { path, readOnly: modal.mountReadOnly };
		modal.mountKey = '';
		modal.mountPath = '';
		modal.mountReadOnly = false;
	}

	function removeMountPoint(key: string) {
		if (modal.host.mountPoints) {
			delete modal.host.mountPoints[key];
			modal.host.mountPoints = { ...modal.host.mountPoints };
		}
	}

	async function saveHost() {
		try {
			if (modal.mode === 'add') {
				await hostsApi.create(modal.host);
			} else {
				await hostsApi.update(modal.host.id, modal.host);
			}
			closeModal();
			await loadHosts();
		} catch (e) { console.error(e); }
	}

	function deleteHost(id: string, name: string) {
		showConfirm('删除主机', `确定要删除 "${name}" 吗？`, async () => {
			try { await hostsApi.delete(id); await loadHosts(); } catch (e) { console.error(e); }
		});
	}

	async function testHost(id: string) {
		testLoading = true;
		testResult = null;
		try {
			const result = await hostsApi.test(id);
			testResult = { hostId: id, status: result.status, message: result.message };
		} catch (e) {
			testResult = { hostId: id, status: 'error', message: String(e) };
		} finally { testLoading = false; }
	}

	function getDriverLabel(d: string) { return d === 'tcp' ? 'TCP' : d === 'ssh' ? 'SSH' : 'Socket'; }
	function getDriverColor(d: string) { return d === 'ssh' ? 'text-green-400' : d === 'tcp' ? 'text-blue-400' : 'text-orange-400'; }

	const thClass = 'px-3 py-1.5 text-left text-[11px] font-medium uppercase tracking-wider text-text-muted border-b border-border-secondary select-none whitespace-nowrap';
	const tdClass = 'px-3 py-2 text-[13px] text-text-primary border-b border-border-secondary/50';
</script>

<div class="flex h-full flex-col bg-surface-primary">
	<div class="flex items-center justify-between border-b border-border-secondary px-4 py-3">
		<h1 class="text-base font-semibold text-text-primary">主机 <span class="ml-1 text-sm font-normal text-text-muted">({hostList.length})</span></h1>
		<div class="flex items-center gap-2">
			<Button variant="secondary" size="sm" onclick={openAdd} title="添加主机"><Plus size={14} /></Button>
			<Button variant="secondary" size="sm" onclick={loadHosts} title="刷新"><RefreshCw size={14} /></Button>
		</div>
	</div>
	<div class="flex-1 overflow-auto">
		{#if loading}
			<div class="flex items-center justify-center py-12"><Spinner size="lg" /></div>
		{:else if hostList.length === 0}
			<div class="flex flex-col items-center gap-2 py-12 text-text-muted">
				<span class="text-sm">暂无主机配置</span>
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
					{#each hostList as host (host.id)}
						<tr class="transition-colors hover:bg-surface-secondary">
							<td class="{tdClass}">
								<div class="flex items-center gap-2">
									{#if hostsConfig.default === host.id}
										<span class="h-2 w-2 rounded-full bg-green-500 shrink-0"></span>
									{:else}
										<span class="h-2 w-2 rounded-full bg-gray-500 shrink-0"></span>
									{/if}
									<span class="font-medium">{host.name}</span>
									<span class="text-[11px] {getDriverColor(host.driver)}">{getDriverLabel(host.driver)}</span>
								</div>
							</td>
							<td class="{tdClass} font-mono text-[12px] text-text-secondary">{host.endpoint}</td>
							<td class="{tdClass}">
								{#if testResult && testResult.hostId === host.id}
									<span class="text-[11px] {testResult.status === 'ok' ? 'text-green-400' : 'text-red-400'}">{testResult.message}</span>
								{:else}
									<span class="text-[11px] text-text-muted">未检测</span>
								{/if}
							</td>
							<td class="{tdClass}">
								<div class="flex flex-wrap gap-1">
									{#each (host.tags || []) as tag}
										<span class="inline-flex items-center rounded bg-surface-tertiary px-1.5 py-0.5 text-[10px] text-text-secondary">{tag}</span>
									{/each}
								</div>
							</td>
							<td class="{tdClass} text-text-secondary text-[12px]">
								{#if host.mountPoints}
									{Object.keys(host.mountPoints).length} 目录
								{:else}
									0 目录
								{/if}
							</td>
							<td class="{tdClass}">
								<div class="flex justify-end gap-1">
									<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary" onclick={() => testHost(host.id)} title="测试连接" disabled={testLoading}>
										<Plug size={13} />
									</button>
									<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary" onclick={() => openEdit(host)} title="编辑">
										<Pencil size={13} />
									</button>
									<button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded text-red-400 transition-colors hover:bg-red-500/10" onclick={() => deleteHost(host.id, host.name)} title="删除">
										<Trash2 size={13} />
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

<!-- Add/Edit Modal -->
{#if modal.open}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
		<div class="flex max-h-[85vh] w-[700px] flex-col rounded-lg bg-surface-primary shadow-xl border border-border-secondary">
			<div class="flex items-center justify-between border-b border-border-secondary px-4 py-3">
				<h3 class="text-sm font-semibold text-text-primary">{modal.mode === 'add' ? '添加主机' : '编辑主机 - ' + modal.host.name}</h3>
				<button type="button" class="text-text-muted hover:text-text-primary" onclick={closeModal}><X size={16} /></button>
			</div>
			<div class="flex-1 overflow-auto p-4 space-y-4">
				<!-- Basic info -->
				<div class="grid grid-cols-2 gap-3">
					<div>
						<label class="mb-1 block text-[11px] text-text-muted">主机ID</label>
						<input type="text" bind:value={modal.host.id} disabled={modal.mode === 'edit'}
							placeholder="my-nas" class="w-full rounded border border-border-secondary bg-surface-secondary px-3 py-1.5 text-xs text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none disabled:opacity-50" />
					</div>
					<div>
						<label class="mb-1 block text-[11px] text-text-muted">显示名称</label>
						<input type="text" bind:value={modal.host.name} placeholder="主NAS"
							class="w-full rounded border border-border-secondary bg-surface-secondary px-3 py-1.5 text-xs text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none" />
					</div>
				</div>

				<!-- Connection -->
				<div class="grid grid-cols-2 gap-3">
					<div>
						<label class="mb-1 block text-[11px] text-text-muted">连接方式</label>
						<select bind:value={modal.host.driver}
							class="w-full rounded border border-border-secondary bg-surface-secondary px-3 py-1.5 text-xs text-text-primary focus:border-border-focus focus:outline-none">
							<option value="ssh">SSH</option>
							<option value="tcp">TCP</option>
							<option value="socket">Socket</option>
						</select>
					</div>
					<div>
						<label class="mb-1 block text-[11px] text-text-muted">
							{modal.host.driver === 'ssh' ? 'user@host:port' : modal.host.driver === 'tcp' ? 'host:port' : '/var/run/docker.sock'}
						</label>
						<input type="text" bind:value={modal.host.endpoint}
							placeholder={modal.host.driver === 'ssh' ? 'root@192.168.1.100' : modal.host.driver === 'tcp' ? '192.168.1.100:2375' : '/var/run/docker.sock'}
							class="w-full rounded border border-border-secondary bg-surface-secondary px-3 py-1.5 text-xs text-text-primary font-mono placeholder:text-text-muted focus:border-border-focus focus:outline-none" />
					</div>
				</div>

				<!-- SSH Key -->
				{#if modal.host.driver === 'ssh'}
					<div>
						<button type="button" class="flex items-center gap-1 text-[11px] text-blue-400 hover:text-blue-300"
							onclick={() => modal.showSsh = !modal.showSsh}>
							{#if modal.showSsh}<ChevronUp size={12} />{:else}<ChevronDown size={12} />{/if}
							SSH密钥配置帮助
						</button>
						{#if modal.showSsh}
							<div class="mt-2 rounded border border-border-secondary bg-surface-secondary p-3 space-y-2 text-[11px] text-text-secondary">
								<p class="text-text-muted font-medium">1. 生成密钥对：</p>
								<div class="flex items-center gap-2 rounded bg-black/30 px-2 py-1 font-mono">
									<span class="flex-1 truncate">ssh-keygen -t ed25519 -C "boxbox" -f ~/.ssh/boxbox_ed25519 -N ""</span>
									<button type="button" class="shrink-0 text-text-muted hover:text-text-primary" onclick={() => { navigator.clipboard.writeText('ssh-keygen -t ed25519 -C "boxbox" -f ~/.ssh/boxbox_ed25519 -N ""'); sshCopied['gen'] = true; setTimeout(() => sshCopied['gen'] = false, 2000); }}>
										{#if sshCopied['gen']}<Check size={12} class="text-green-400" />{:else}<Copy size={12} />{/if}
									</button>
								</div>
								<p class="text-text-muted font-medium">2. 复制公钥到宿主机：</p>
								<div class="flex items-center gap-2 rounded bg-black/30 px-2 py-1 font-mono">
									<span class="flex-1 truncate">ssh-copy-id -i ~/.ssh/boxbox_ed25519.pub USER@HOST</span>
									<button type="button" class="shrink-0 text-text-muted hover:text-text-primary" onclick={() => { navigator.clipboard.writeText('ssh-copy-id -i ~/.ssh/boxbox_ed25519.pub USER@HOST'); sshCopied['copy'] = true; setTimeout(() => sshCopied['copy'] = false, 2000); }}>
										{#if sshCopied['copy']}<Check size={12} class="text-green-400" />{:else}<Copy size={12} />{/if}
									</button>
								</div>
								<p class="text-text-muted font-medium">3. 验证连接：</p>
								<div class="flex items-center gap-2 rounded bg-black/30 px-2 py-1 font-mono">
									<span class="flex-1 truncate">ssh -i ~/.ssh/boxbox_ed25519 USER@HOST docker info</span>
									<button type="button" class="shrink-0 text-text-muted hover:text-text-primary" onclick={() => { navigator.clipboard.writeText('ssh -i ~/.ssh/boxbox_ed25519 USER@HOST docker info'); sshCopied['test'] = true; setTimeout(() => sshCopied['test'] = false, 2000); }}>
										{#if sshCopied['test']}<Check size={12} class="text-green-400" />{:else}<Copy size={12} />{/if}
									</button>
								</div>
								<div>
									<label class="mb-1 block text-[11px] text-text-muted">私钥路径（可选，默认 ~/.ssh/id_rsa）</label>
									<input type="text" bind:value={modal.host.sshKey} placeholder="~/.ssh/boxbox_ed25519"
										class="w-full rounded border border-border-secondary bg-surface-secondary px-3 py-1.5 text-xs text-text-primary font-mono placeholder:text-text-muted focus:border-border-focus focus:outline-none" />
								</div>
							</div>
						{/if}
					</div>
				{/if}

				<!-- Tags -->
				<div>
					<label class="mb-1 block text-[11px] text-text-muted">标签（逗号分隔）</label>
					<input type="text" value={(modal.host.tags || []).join(', ')}
						onchange={(e) => { modal.host.tags = (e.target as HTMLInputElement).value.split(',').map(s => s.trim()).filter(Boolean); }}
						placeholder="home, nas, prod"
						class="w-full rounded border border-border-secondary bg-surface-secondary px-3 py-1.5 text-xs text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none" />
				</div>

				<!-- Mount Points -->
				<div>
					<label class="mb-2 block text-[11px] text-text-muted font-medium">挂载目录</label>
					<div class="space-y-1.5 mb-2">
						{#each Object.entries(modal.host.mountPoints || {}) as [key, mp]}
							<div class="flex items-center gap-2 rounded border border-border-secondary bg-surface-secondary px-3 py-1.5">
								<span class="text-[12px] font-medium text-text-primary min-w-[80px]">{key}</span>
								<span class="flex-1 text-[11px] text-text-secondary font-mono truncate">{mp.path}</span>
								{#if mp.readOnly}
									<span class="text-[10px] text-text-muted">只读</span>
								{/if}
								<button type="button" class="text-text-muted hover:text-red-400" onclick={() => removeMountPoint(key)}>
									<X size={12} />
								</button>
							</div>
						{/each}
					</div>
					<div class="flex items-center gap-2">
						<input type="text" bind:value={modal.mountKey} placeholder="名称 (如: docker)"
							class="w-28 rounded border border-border-secondary bg-surface-secondary px-2 py-1 text-[11px] text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none" />
						<input type="text" bind:value={modal.mountPath} placeholder="/opt/docker"
							class="flex-1 rounded border border-border-secondary bg-surface-secondary px-2 py-1 text-[11px] text-text-primary font-mono placeholder:text-text-muted focus:border-border-focus focus:outline-none" />
						<label class="flex items-center gap-1 text-[11px] text-text-muted cursor-pointer">
							<input type="checkbox" bind:checked={modal.mountReadOnly} class="rounded" /> 只读
						</label>
						<button type="button" class="rounded bg-surface-tertiary px-2 py-1 text-[11px] text-text-primary hover:bg-surface-secondary" onclick={addMountPoint}>添加</button>
					</div>
				</div>
			</div>
			<div class="flex items-center justify-end gap-2 border-t border-border-secondary px-4 py-3">
				<Button variant="secondary" size="sm" onclick={closeModal}>取消</Button>
				<Button variant="primary" size="sm" onclick={saveHost} disabled={!modal.host.id || !modal.host.name || !modal.host.endpoint}>
					{modal.mode === 'add' ? '添加' : '保存'}
				</Button>
			</div>
		</div>
	</div>
{/if}
