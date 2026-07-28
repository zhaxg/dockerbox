<script lang="ts">
import { t, setLocale, getLocale } from '$lib/i18n/index.svelte';
const tHostsAddhost = $derived(t("hosts.addHost"));
const tHostsNohosts = $derived(t("hosts.noHosts"));
const tHostsNomatch = $derived(t("hosts.noMatch"));
const tHostsTestconnection = $derived(t("hosts.testConnection"));
const tCommonCancel = $derived(t("common.cancel"));
const tCommonDelete = $derived(t("common.delete"));
const tCommonEdit = $derived(t("common.edit"));
const tContainersConfirm = $derived(t("containers.confirm"));
const tHostsChecking = $derived(t("hosts.checking"));
const tHostsOffline = $derived(t("hosts.offline"));
const tHostsOnline = $derived(t("hosts.online"));
const tHostsRefresh = $derived(t("hosts.refresh"));
const tHostsSearch = $derived(t("hosts.search"));
const tHostsTitle = $derived(t("hosts.title"));
const tHostsConnectsuccess = $derived(t("hosts.connectSuccess"));
const tHostsConnectfailed = $derived(t("hosts.connectFailed"));
const tHostsDeletehost = $derived(t("hosts.deleteHost"));
const tHostsDeleteconfirm = $derived(t("hosts.deleteConfirm"));
const tHostmodalMaindirpath = $derived(t("hostModal.mainDirPath"));
const tTableName = $derived(t("table.name"));
const tTableEndpoint = $derived(t("table.endpoint"));
const tTableStatus = $derived(t("table.status"));
const tTableTags = $derived(t("table.tags"));
const tTableDocker = $derived(t("table.docker"));
const tTableActions = $derived(t("table.actions"));
	import { onMount } from 'svelte';
	import { Spinner, Button, Badge } from '$lib/components/ui';
	import { hostsApi, type DockerHost, type DockerHostsConfig } from '$lib/api/hosts';
	import {
		Plus,
		RefreshCw,
		Trash2,
		Pencil,
		Plug,
		X,
		Copy,
		Check,
		Key,
		Terminal,
		MessageSquare,
		Search,
		FileUp
	} from 'lucide-svelte';
	import HostModal from '$lib/components/HostModal.svelte';
	
	let hostsConfig = $state<DockerHostsConfig>({ default: '', hosts: {} });
	let loading = $state(true);
	let searchQuery = $state('');
	let confirmDialog = $state<{
		open: boolean;
		title: string;
		message: string;
		onConfirm: () => void;
	}>({
		open: false,
		title: '',
		message: '',
		onConfirm: () => {}
	});
	let testLoading = $state(false);
	let copied = $state<Record<string, boolean>>({});
	let genKeyLoading = $state(false);
	let hostStats = $state<
		Record<string, { status: string; total: number; running: number; stopped: number }>
	>({});
	let toastMsg = $state('');
	let toastType = $state<'ok' | 'err'>('ok');

	let modal = $state<{
		open: boolean;
		mode: 'add' | 'edit';
		host: any;
		mountKey: string;
		mountPath: string;
		mountReadOnly: boolean;
		dockerDirKey: string;
		isDefault: boolean;
	}>({
		open: false,
		mode: 'add',
		host: {
			id: '',
			name: '',
			driver: 'socket',
			endpoint: '/var/run/docker.sock',
			sshKey: '',
			sshPubKey: '',
			tags: [],
			mountPoints: {},
			key: ''
		},
		mountKey: '',
		mountPath: '',
		mountReadOnly: false,
		dockerDirKey: '',
		isDefault: false
	});

	const hostList = $derived(
		Object.entries(hostsConfig.hosts || {}).map(([key, host]) => ({ key, ...host }))
	);
	const filteredHostList = $derived(
		searchQuery.trim()
			? hostList.filter(
					(h) =>
						h.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
						h.endpoint.toLowerCase().includes(searchQuery.toLowerCase())
				)
			: hostList
	);
	onMount(loadHosts);

	async function loadHosts() {
		loading = true;
		try {
			hostsConfig = await hostsApi.list();
			if (!hostsConfig.hosts) hostsConfig.hosts = {};
			for (const id of Object.keys(hostsConfig.hosts)) fetchHostStats(id);
		} catch (e) {
			console.error(e);
		} finally {
			loading = false;
		}
	}

	async function fetchHostStats(id: string) {
		try {
			const token = localStorage.getItem('accessToken') || '';
			const resp = await fetch(`/api/v1/hosts/${id}/stats`, {
				headers: { Authorization: 'Bearer ' + token }
			});
			if (resp.ok) hostStats[id] = await resp.json();
		} catch (e) {
			hostStats[id] = { status: 'offline', total: 0, running: 0, stopped: 0 };
		}
	}

	function showToast(msg: string, type: 'ok' | 'err' = 'ok') {
		toastMsg = msg;
		toastType = type;
		setTimeout(() => (toastMsg = ''), 4000);
	}

	function showConfirm(title: string, message: string, onConfirm: () => void) {
		confirmDialog = { open: true, title, message, onConfirm };
	}
	function closeConfirm() {
		confirmDialog.open = false;
	}

	function openAdd() {
		modal = {
			open: true,
			mode: 'add',
			host: {
				id: 'host-' + Date.now().toString(36),
				name: '',
				driver: 'socket',
				endpoint: '/var/run/docker.sock',
				sshKey: '',
				tags: [],
				mountPoints: { docker: { path: '/var/docker', readOnly: false } }
			},
			mountKey: '',
			mountPath: '',
			mountReadOnly: false,
			dockerDirKey: 'docker',
			isDefault: false
		};
	}

	function openEdit(host: any) {
		const mp = host.mountPoints || {};
		// Ensure docker key exists
		if (!mp.docker) mp.docker = { path: '', readOnly: false };
		modal = {
			open: true,
			mode: 'edit',
			host: { ...host, id: host.key, mountPoints: { ...mp }, tags: [...(host.tags || [])] },
			mountKey: '',
			mountPath: '',
			mountReadOnly: false,
			dockerDirKey: 'docker',
			isDefault: hostsConfig.default === host.key
		};
	}

	function closeModal() {
		modal.open = false;
	}

	function addMountPoint() {
		const key = modal.mountKey.trim(),
			path = modal.mountPath.trim();
		if (!key || !path) return;
		const mp = modal.host.mountPoints || {};
		modal.host = { ...modal.host, mountPoints: { ...mp, [key]: { path, readOnly: modal.mountReadOnly } } };
		modal.mountKey = '';
		modal.mountPath = '';
		modal.mountReadOnly = false;
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
				method: 'POST',
				headers: { Authorization: 'Bearer ' + (localStorage.getItem('accessToken') || '') }
			}).then((r) => r.json());
			if (result.private_key) {
				modal.host.sshKey = result.private_key;
				modal.host.sshPubKey = result.public_key;
			}
		} catch (e) {
			console.error(e);
		} finally {
			genKeyLoading = false;
		}
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
			ta.value = text;
			ta.style.position = 'fixed';
			ta.style.left = '-9999px';
			document.body.appendChild(ta);
			ta.select();
			document.execCommand('copy');
			document.body.removeChild(ta);
		}
		copied[key] = true;
		setTimeout(() => (copied[key] = false), 2000);
	}

	function getCopyCmd() {
		return `mkdir -p ~/.ssh && chmod 700 ~/.ssh && echo '${modal.host.sshPubKey || ''}' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys`;
	}

	async function doSave(): Promise<string | null> {
		// Validate docker directory path
		const dockerMP = modal.host.mountPoints?.docker;
		if (!dockerMP || !dockerMP.path?.trim()) {
			showToast(tHostmodalMaindirpath, 'err');
			return null;
		}
		const saveData = { ...modal.host, isDefault: modal.isDefault };
		try {
			if (modal.mode === 'add') {
				const result = await hostsApi.create<{id: string, host: any}>(saveData);
				if (result?.id) modal.host.id = result.id;
			} else {
				await hostsApi.update(modal.host.id, saveData);
			}
			hostsConfig.default = modal.isDefault
				? modal.host.id
				: hostsConfig.default === modal.host.id
					? ''
					: hostsConfig.default;
			await loadHosts();
			return modal.host.id;
		} catch (e) {
			console.error(e);
			return null;
		}
	}

	function saveAndClose() {
		doSave().then(() => closeModal());
	}

	async function saveAndTest() {
		const savedId = await doSave();
		if (!savedId) return;
		testLoading = true;
		try {
			const result = await hostsApi.test(savedId);
			showToast(
				result.status === 'ok' ? tHostsConnectsuccess + ': ' + result.message : tHostsConnectfailed + ': ' + result.message,
				result.status === 'ok' ? 'ok' : 'err'
			);
		} catch (e) {
			showToast(tHostsConnectfailed + ': ' + String(e), 'err');
		} finally {
			testLoading = false;
		}
	}

	function deleteHost(id: string, name: string) {
		showConfirm(tHostsDeletehost, tHostsDeleteconfirm, async () => {
			try {
				await hostsApi.delete(id);
				await loadHosts();
			} catch (e) {
				console.error(e);
			}
		});
	}

	async function testHost(id: string) {
		testLoading = true;
		try {
			const result = await hostsApi.test(id);
			showToast(
				result.status === 'ok' ? tHostsConnectsuccess + ': ' + result.message : tHostsConnectfailed + ': ' + result.message,
				result.status === 'ok' ? 'ok' : 'err'
			);
		} catch (e) {
			showToast(tHostsConnectfailed + ': ' + String(e), 'err');
		} finally {
			testLoading = false;
		}
	}

	function getDriverLabel(d: string) {
		return d === 'ssh' ? 'SSH' : 'Socket';
	}
	function getDriverColor(d: string) {
		return d === 'ssh' ? 'text-green-400' : 'text-orange-400';
	}

	const thClass =
		'px-3 py-1.5 text-left text-[11px] font-medium uppercase tracking-wider text-text-muted border-b border-border-secondary select-none whitespace-nowrap';
	const tdClass = 'px-3 py-2 text-[13px] text-text-primary border-b border-border-secondary/50';

	// i18n
</script>

<div class="flex h-full flex-col bg-surface-primary">
	<div class="flex items-center justify-between border-b border-border-secondary px-4 py-3">
		<h1 class="text-base font-semibold text-text-primary">{tHostsTitle} <Badge>{hostList.length}</Badge></h1>
		<div class="flex items-center gap-2">
			<div class="relative">
				<Search size={14} class="absolute top-1/2 left-2.5 -translate-y-1/2 text-text-muted" />
				<input
					type="text"
					bind:value={searchQuery}
					placeholder={tHostsSearch}
					class="h-7 w-48 rounded border border-border-secondary bg-surface-secondary pr-2 pl-8 text-xs text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none"
				/>
			</div>
			<Button variant="secondary" size="sm" onclick={openAdd} title={tHostsAddhost}
				><Plus size={14} /></Button
			>
			<Button variant="secondary" size="sm" onclick={loadHosts} title={tHostsRefresh}
				><RefreshCw size={14} /></Button
			>
		</div>
	</div>
	<div class="flex-1 overflow-auto">
		{#if loading}
			<div class="flex items-center justify-center py-12"><Spinner size="lg" /></div>
		{:else if hostList.length === 0}
			<div class="flex flex-col items-center gap-2 py-12 text-text-muted">
				<span class="text-sm">{searchQuery ? tHostsNomatch : tHostsNohosts}</span>
				<Button variant="primary" size="sm" onclick={openAdd}
					><Plus size={14} class="mr-1" /> {tHostsAddhost}</Button
				>
			</div>
		{:else}
			<table class="w-full min-w-[900px] border-collapse text-[13px] leading-5">
				<thead
					><tr>
						<th class={thClass}>{tTableName}</th>
						<th class={thClass}>{tTableEndpoint}</th>
						<th class={thClass}>{tTableStatus}</th>
						<th class={thClass}>{tTableTags}</th>
						<th class={thClass}>{tTableDocker}</th>
						<th class="{thClass} text-right">{tTableActions}</th>
					</tr></thead
				>
				<tbody>
					{#each filteredHostList as host (host.key)}
						<tr class="transition-colors hover:bg-surface-secondary">
							<td class={tdClass}>
								<div class="flex items-center gap-2">
									{#if hostStats[host.key]}
										<span
											class="h-2 w-2 shrink-0 rounded-full {hostStats[host.key].status === 'online'
												? 'bg-green-500'
												: 'bg-red-500'}"
										></span>
									{:else}<span class="h-2 w-2 shrink-0 rounded-full bg-gray-500"></span>{/if}
									<span class="font-medium">{host.name}</span>
									<span class="text-[11px] {getDriverColor(host.driver)}"
										>{getDriverLabel(host.driver)}</span
									>
								</div>
							</td>
							<td class="{tdClass} font-mono text-[12px] text-text-secondary">{host.endpoint}</td>
							<td class={tdClass}>
								{#if hostStats[host.key]}
									<span
										class="text-[11px] {hostStats[host.key].status === 'online'
											? 'text-green-400'
											: 'text-red-400'}"
									>
										{hostStats[host.key].status === 'online' ? tHostsOnline : tHostsOffline}
									</span>
								{:else}<span class="text-[11px] text-text-muted">{tHostsChecking}</span>{/if}
							</td>
							<td class={tdClass}>
								<div class="flex flex-wrap gap-1">
									{#each host.tags || [] as tag}
										<span
											class="inline-flex items-center rounded bg-surface-tertiary px-1.5 py-0.5 text-[10px] text-text-secondary"
											>{tag}</span
										>
									{/each}
								</div>
							</td>
							<td class="{tdClass} text-[12px] text-text-secondary">
								{#if hostStats[host.key]}{hostStats[host.key].running}/{hostStats[host.key]
										.total}{:else}-{/if}
							</td>
							<td class={tdClass}>
								<div class="flex items-center justify-end gap-1">
									{#if hostsConfig.default === host.key}
										<Check size={14} class="shrink-0 text-green-400" />
									{/if}
									<button
										type="button"
										class="inline-flex h-6 w-6 items-center justify-center rounded text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary"
										onclick={() => testHost(host.key)}
										title={tHostsTestconnection}
										disabled={testLoading}><Plug size={13} /></button
									>
									<button
										type="button"
										class="inline-flex h-6 w-6 items-center justify-center rounded text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary"
										onclick={() => openEdit(host)}
										title={tCommonEdit}><Pencil size={13} /></button
									>
									<button
										type="button"
										class="inline-flex h-6 w-6 items-center justify-center rounded text-red-400 transition-colors hover:bg-red-500/10"
										onclick={() => deleteHost(host.key, host.name)}
										title={tCommonDelete}><Trash2 size={13} /></button
									>
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
	<div
		class="fixed right-4 bottom-4 z-[70] max-w-md rounded-lg border px-4 py-3 shadow-xl {toastType ===
		'ok'
			? 'border-green-500/30 bg-green-500/10 text-green-400'
			: 'border-red-500/30 bg-red-500/10 text-red-400'}"
	>
		<div class="flex items-center gap-2 text-[12px]">
			<MessageSquare size={14} /><span>{toastMsg}</span>
		</div>
	</div>
{/if}

<!-- Confirm Dialog -->
{#if confirmDialog.open}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
		<div class="w-96 rounded-lg border border-border-secondary bg-surface-primary p-6 shadow-xl">
			<h2 class="mb-2 text-lg font-semibold text-text-primary">{confirmDialog.title}</h2>
			<p class="mb-6 text-sm text-text-secondary">{confirmDialog.message}</p>
			<div class="flex justify-end gap-2">
				<Button variant="secondary" onclick={closeConfirm}>{tCommonCancel}</Button>
				<Button
					variant="danger"
					onclick={() => {
						confirmDialog.onConfirm();
						closeConfirm();
					}}>{tContainersConfirm}</Button
				>
			</div>
		</div>
	</div>
{/if}

<HostModal
	bind:open={modal.open}
	bind:mode={modal.mode}
	bind:host={modal.host}
	bind:isDefault={modal.isDefault}
	{testLoading}
	{genKeyLoading}
	{copied}
	onClose={closeModal}
	onSave={saveAndClose}
	onSaveAndTest={saveAndTest}
	onGenKeyPair={genKeyPair}
	onKeyFileUpload={handleKeyFileUpload}
	onCopyText={copyText}
	onGetCopyCmd={getCopyCmd}
	onAddMountPoint={addMountPoint}
	onRemoveMountPoint={removeMountPoint}
	bind:mountKey={modal.mountKey}
	bind:mountPath={modal.mountPath}
	bind:mountReadOnly={modal.mountReadOnly}
/>
