<script lang="ts">
	import { Spinner, Button } from '$lib/components/ui';
	import { X, Key, Copy, Check, Terminal, FileUp, Plug } from 'lucide-svelte';
	import { t, setLocale, getLocale } from '$lib/i18n/index.svelte';
	const tHostmodalAddhost = $derived(t("hostModal.addHost"));
	const tHostmodalEdithost = $derived(t("hostModal.editHost"));
	const tHostmodalDisplayname = $derived(t("hostModal.displayName"));
	const tHostmodalDefaulthost = $derived(t("hostModal.defaultHost"));
	const tHostmodalConnectiontype = $derived(t("hostModal.connectionType"));
	const tHostmodalKeypair = $derived(t("hostModal.keyPair"));
	const tHostmodalUploadkey = $derived(t("hostModal.uploadKey"));
	const tHostmodalGeneratekey = $derived(t("hostModal.generateKey"));
	const tHostmodalPrivatekey = $derived(t("hostModal.privateKey"));
	const tHostmodalPublickey = $derived(t("hostModal.publicKey"));
	const tHostmodalClickgenerate = $derived(t("hostModal.clickGenerate"));
	const tHostmodalCopied = $derived(t("hostModal.copied"));
	const tHostmodalKeycommand = $derived(t("hostModal.keyCommand"));
	const tHostmodalTags = $derived(t("hostModal.tags"));
	const tHostmodalMountpoints = $derived(t("hostModal.mountPoints"));
	const tHostmodalDockermaindir = $derived(t("hostModal.dockerMainDir"));
	const tHostmodalReadonly = $derived(t("hostModal.readOnly"));
	const tHostmodalName = $derived(t("hostModal.name"));
	const tHostmodalAdd = $derived(t("hostModal.add"));
	const tHostmodalTestconnection = $derived(t("hostModal.testConnection"));
	const tCommonCancel = $derived(t("common.cancel"));
	const tCommonSave = $derived(t("common.save"));
	const tCommonCommand = $derived(t("common.command"));
	
	let {
		open = false,
		mode = 'add',
		host = $bindable({
			name: '',
			driver: 'socket',
			endpoint: '/var/run/docker.sock',
			sshKey: '',
			sshPubKey: '',
			tags: [],
			mountPoints: {}
		}),
		isDefault = $bindable(false),
		testLoading = false,
		genKeyLoading = false,
		copied = {},
		onClose,
		onSave,
		onSaveAndTest,
		onGenKeyPair,
		onKeyFileUpload,
		onCopyText,
		onGetCopyCmd,
		onAddMountPoint,
		onRemoveMountPoint,
		mountKey = $bindable(''),
		mountPath = $bindable(''),
		mountReadOnly = $bindable(false)
	}: {
		open: boolean;
		mode: 'add' | 'edit';
		host: any;
		isDefault: boolean;
		testLoading: boolean;
		genKeyLoading: boolean;
		copied: Record<string, boolean>;
		onClose: () => void;
		onSave: () => void;
		onSaveAndTest: () => void;
		onGenKeyPair: () => void;
		onKeyFileUpload: (e: Event) => void;
		onCopyText: (text: string, key: string) => void;
		onGetCopyCmd: () => string;
		onAddMountPoint: () => void;
		onRemoveMountPoint: (key: string) => void;
		mountKey: string;
		mountPath: string;
		mountReadOnly: boolean;
	} = $props();

	let keyFileInput: HTMLInputElement | null = $state(null);

	function handleSocketDefault() {
		if (host.driver === 'socket' && !host.endpoint) {
			host.endpoint = '/var/run/docker.sock';
		}
	}

	function handleTagsChange(e: Event) {
		host.tags = (e.target as HTMLInputElement).value
			.split(',')
			.map((s) => s.trim())
			.filter(Boolean);
	}

	function handleDockerPathChange(e: Event) {
		const mp = host.mountPoints || {};
		host = {
			...host,
			mountPoints: {
				...mp,
				docker: { path: (e.target as HTMLInputElement).value, readOnly: false }
			}
		};
	}

	function handleMountPathChange(key: string, e: Event) {
		const mp = host.mountPoints || {};
		const entry = mp[key] || { path: '', readOnly: false };
		host = {
			...host,
			mountPoints: { ...mp, [key]: { ...entry, path: (e.target as HTMLInputElement).value } }
		};
	}

	// i18n
</script>

{#if open}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
		<div
			class="flex max-h-[85vh] w-[700px] flex-col rounded-lg border border-border-secondary bg-surface-primary shadow-xl"
		>
			<div class="flex items-center justify-between border-b border-border-secondary px-4 py-3">
				<h3 class="text-sm font-semibold text-text-primary">
					{mode === 'add' ? tHostmodalAddhost : tHostmodalEdithost + ' - ' + host.name}
				</h3>
				<button type="button" class="text-text-muted hover:text-text-primary" onclick={onClose}
					><X size={16} /></button
				>
			</div>
			<div class="flex-1 space-y-4 overflow-auto p-4">
				<!-- i18n: name + default -->
				<div class="flex items-end gap-3">
					<div class="flex-1">
						<label class="mb-1 block text-[11px] text-text-muted"
							>{tHostmodalDisplayname} <span class="text-red-400">*</span></label
						>
						<input
							type="text"
							bind:value={host.name}
							placeholder={tHostmodalDisplayname}
							class="w-full rounded border border-border-secondary bg-surface-secondary px-3 py-1.5 text-xs text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none"
						/>
					</div>
					<label
						class="flex cursor-pointer items-center gap-1.5 pb-1.5 text-[11px] whitespace-nowrap text-text-muted"
					>
						<input type="checkbox" bind:checked={isDefault} class="rounded accent-green-500" /> {tHostmodalDefaulthost}
					</label>
				</div>
				<!-- i18n: connection + endpoint -->
				<div class="grid grid-cols-3 gap-3">
					<div>
						<label class="mb-1 block text-[11px] text-text-muted">{tHostmodalConnectiontype}</label>
						<select
							bind:value={host.driver}
							onchange={handleSocketDefault}
							class="w-full rounded border border-border-secondary bg-surface-secondary px-3 py-1.5 text-xs text-text-primary focus:border-border-focus focus:outline-none"
						>
							<option value="socket">Socket</option>
							<option value="ssh">SSH</option>
						</select>
					</div>
					<div class="col-span-2">
						<label class="mb-1 block text-[11px] text-text-muted"
							>{host.driver === 'ssh' ? 'user@host:port' : '/var/run/docker.sock'}</label
						>
						<input
							type="text"
							bind:value={host.endpoint}
							placeholder={host.driver === 'ssh' ? 'root@192.168.1.100:22' : '/var/run/docker.sock'}
							class="w-full rounded border border-border-secondary bg-surface-secondary px-3 py-1.5 font-mono text-xs text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none"
						/>
					</div>
				</div>
				<!-- SSH Key -->
				{#if host.driver === 'ssh'}
					<div class="space-y-3 rounded border border-border-secondary bg-surface-secondary p-3">
						<div class="flex items-center justify-between">
							<span class="flex items-center gap-1 text-[11px] font-medium text-text-muted"
								><Key size={12} /> ED25519 {tHostmodalKeypair}</span
							>
							<div class="flex items-center gap-1.5">
								<input
									type="file"
									bind:this={keyFileInput}
									accept=".pem,.key,id_ed25519,id_rsa,*.pem,*.key"
									class="hidden"
									onchange={onKeyFileUpload}
								/>
								<Button variant="secondary" size="sm" onclick={() => keyFileInput?.click()}
									><FileUp size={12} class="mr-1" />{tHostmodalUploadkey}</Button
								>
								<Button
									variant="secondary"
									size="sm"
									onclick={onGenKeyPair}
									disabled={genKeyLoading}
									>{#if genKeyLoading}<Spinner size={12} />{:else}{tHostmodalGeneratekey}{/if}</Button
								>
							</div>
						</div>
						<div>
							<label class="mb-1 block text-[10px] text-text-muted">{tHostmodalPrivatekey}</label>
							<textarea
								bind:value={host.sshKey}
								rows={3}
								placeholder={tHostmodalClickgenerate}
								class="w-full resize-none rounded border border-border-secondary bg-black/30 px-2 py-1 font-mono text-[11px] text-green-400 placeholder:text-text-muted focus:border-border-focus focus:outline-none"
							></textarea>
						</div>
						<div>
							<label class="mb-1 block text-[10px] text-text-muted">{tHostmodalPublickey}</label>
							<input
								type="text"
								bind:value={host.sshPubKey}
								placeholder={tHostmodalClickgenerate}
								class="w-full rounded border border-border-secondary bg-black/30 px-2 py-1 font-mono text-[11px] text-green-400 placeholder:text-text-muted focus:border-border-focus focus:outline-none"
							/>
						</div>
						<div class="flex items-center gap-1 border-t border-border-secondary pt-2">
							<button
								type="button"
								class="inline-flex items-center gap-1 rounded px-2 py-1 text-[10px] text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary"
								onclick={() => onCopyText(host.sshKey || '', 'priv')}
								disabled={!host.sshKey}
							>
								{#if copied['priv']}<Check size={10} class="text-green-400" />{tHostmodalCopied}{:else}<Copy
										size={10}
									/>{tHostmodalPrivatekey}{/if}
							</button>
							<button
								type="button"
								class="inline-flex items-center gap-1 rounded px-2 py-1 text-[10px] text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary"
								onclick={() => onCopyText(host.sshPubKey || '', 'pub')}
								disabled={!host.sshPubKey}
							>
								{#if copied['pub']}<Check size={10} class="text-green-400" />{tHostmodalCopied}{:else}<Copy
										size={10}
									/>{tHostmodalPublickey}{/if}
							</button>
							<button
								type="button"
								class="inline-flex items-center gap-1 rounded px-2 py-1 text-[10px] text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary"
								onclick={() => onCopyText(onGetCopyCmd(), 'cmd')}
								disabled={!host.sshPubKey || !host.endpoint}
							>
								{#if copied['cmd']}<Check size={10} class="text-green-400" />{tHostmodalCopied}{:else}<Terminal
										size={10}
									/>{tHostmodalKeycommand}{/if}
							</button>
							<button
								type="button"
								class="inline-flex items-center gap-1 rounded px-2 py-1 text-[10px] text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary"
								onclick={() => onCopyText('systemctl enable --now podman.socket', 'podman')}
							>
								{#if copied['podman']}<Check
										size={10}
										class="text-green-400"
									/>{tHostmodalCopied}{:else}<Terminal size={10} />Podman {tCommonCommand}{/if}
							</button>
						</div>
					</div>
				{/if}
				<!-- Tags -->
				<div>
					<label class="mb-1 block text-[11px] text-text-muted">{tHostmodalTags}</label>
					<input
						type="text"
						value={(host.tags || []).join(', ')}
						onchange={handleTagsChange}
						placeholder="home, nas, prod"
						class="w-full rounded border border-border-secondary bg-surface-secondary px-3 py-1.5 text-xs text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none"
					/>
				</div>
				<!-- {tHostmodalMountpoints} -->
				<div>
					<label class="mb-2 block text-[11px] font-medium text-text-muted">{tHostmodalMountpoints}</label>
					<div class="mb-2 space-y-1">
						<div class="flex items-center gap-2 px-1 py-1">
							<input
								type="text"
								value="docker"
								readonly
								class="w-16 shrink-0 cursor-default rounded border border-border-secondary bg-surface-secondary px-2 py-1 text-[11px] text-text-secondary"
							/>
							<input
								type="text"
								value={host.mountPoints?.docker?.path || ''}
								onchange={handleDockerPathChange}
								placeholder="/var/docker"
								class="min-w-0 flex-1 rounded border border-border-secondary bg-surface-secondary px-2 py-1 font-mono text-[11px] text-text-primary focus:border-border-focus focus:outline-none"
							/>
							<span class="w-[170px] shrink-0 whitespace-nowrap text-left text-[11px] font-medium text-green-400"
								>{tHostmodalDockermaindir}</span
							>
						</div>
						{#each Object.entries(host.mountPoints || {}) as [key, mp]}
							{#if key !== 'docker'}
								<div class="flex items-center gap-2 px-1 py-1">
									<input
										type="text"
										value={key}
										readonly
										class="w-16 shrink-0 cursor-default rounded border border-border-secondary bg-surface-secondary px-2 py-1 text-[11px] text-text-secondary"
									/>
									<input
										type="text"
										value={mp.path || ''}
										onchange={(e) => handleMountPathChange(key, e)}
										placeholder="/path"
										class="min-w-0 flex-1 rounded border border-border-secondary bg-surface-secondary px-2 py-1 font-mono text-[11px] text-text-primary focus:border-border-focus focus:outline-none"
									/>
									<div class="flex w-[170px] shrink-0 items-center justify-start gap-1 whitespace-nowrap">
										{#if mp.readOnly}<span class="text-[10px] text-text-muted">{tHostmodalReadonly}</span>{/if}
										<button
											type="button"
											class="text-text-muted hover:text-red-400"
											onclick={() => onRemoveMountPoint(key)}><X size={12} /></button
										>
									</div>
								</div>
							{/if}
						{/each}
					</div>
					<div class="flex items-center gap-2 px-1 py-1">
						<input
							type="text"
							bind:value={mountKey}
							placeholder="{tHostmodalName}"
							class="w-16 shrink-0 rounded border border-border-secondary bg-surface-secondary px-2 py-1 text-[11px] text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none"
						/>
						<input
							type="text"
							bind:value={mountPath}
							placeholder="/opt/docker"
							class="min-w-0 flex-1 rounded border border-border-secondary bg-surface-secondary px-2 py-1 font-mono text-[11px] text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none"
						/>
						<div class="flex w-[170px] shrink-0 items-center gap-1 whitespace-nowrap">
							<label class="flex cursor-pointer items-center gap-1 text-[11px] text-text-muted"
								><input type="checkbox" bind:checked={mountReadOnly} class="rounded" /> {tHostmodalReadonly}</label
							>
							<button
								type="button"
								class="rounded bg-surface-tertiary px-2 py-1 text-[11px] text-text-primary hover:bg-surface-secondary"
								onclick={onAddMountPoint}>{tHostmodalAdd}</button
							>
						</div>
					</div>
				</div>
			</div>
			<div class="flex items-center justify-between border-t border-border-secondary px-4 py-3">
				<Button
					variant="ghost"
					size="sm"
					onclick={onSaveAndTest}
					disabled={testLoading || !host.name || !host.endpoint}
				>
					{#if testLoading}<Spinner size={12} class="mr-1" />{:else}<Plug
							size={12}
							class="mr-1"
						/>{/if}{tHostmodalTestconnection}
				</Button>
				<div class="flex items-center gap-2 px-1 py-1">
					<Button variant="secondary" size="sm" onclick={onClose}>{tCommonCancel}</Button>
					<Button
						variant="primary"
						size="sm"
						onclick={onSave}
						disabled={!host.name || !host.endpoint}
					>
						{mode === 'add' ? tHostmodalAdd : tCommonSave}
					</Button>
				</div>
			</div>
		</div>
	</div>
{/if}
