<script lang="ts">
	import { Spinner, Button } from '$lib/components/ui';
	import { X, Key, Copy, Check, Terminal, FileUp, Plug } from 'lucide-svelte';
	import { _t, setLocale, getLocale } from '$lib/i18n/index.svelte';
	
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
					{mode === 'add' ? tAddHost() : tEditHost() + ' - ' + host.name}
				</h3>
				<button type="button" class="text-text-muted hover:text-text-primary" onclick={onClose}
					><X size={16} /></button
				>
			</div>
			<div class="flex-1 space-y-4 overflow-auto p-4">
				<!-- {$_t('hostModal.name')} + 默认 -->
				<div class="flex items-end gap-3">
					<div class="flex-1">
						<label class="mb-1 block text-[11px] text-text-muted"
							>{$_t('hostModal.displayName')} <span class="text-red-400">*</span></label
						>
						<input
							type="text"
							bind:value={host.name}
							placeholder="主NAS"
							class="w-full rounded border border-border-secondary bg-surface-secondary px-3 py-1.5 text-xs text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none"
						/>
					</div>
					<label
						class="flex cursor-pointer items-center gap-1.5 pb-1.5 text-[11px] whitespace-nowrap text-text-muted"
					>
						<input type="checkbox" bind:checked={isDefault} class="rounded accent-green-500" /> {$_t('hostModal.defaultHost')}
					</label>
				</div>
				<!-- {$_t('hostModal.connectionType')} + 端点 -->
				<div class="grid grid-cols-3 gap-3">
					<div>
						<label class="mb-1 block text-[11px] text-text-muted">{$_t('hostModal.connectionType')}</label>
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
				<!-- SSH 密钥 -->
				{#if host.driver === 'ssh'}
					<div class="space-y-3 rounded border border-border-secondary bg-surface-secondary p-3">
						<div class="flex items-center justify-between">
							<span class="flex items-center gap-1 text-[11px] font-medium text-text-muted"
								><Key size={12} /> ED25519 密钥对</span
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
									><FileUp size={12} class="mr-1" />上传私钥</Button
								>
								<Button
									variant="secondary"
									size="sm"
									onclick={onGenKeyPair}
									disabled={genKeyLoading}
									>{#if genKeyLoading}<Spinner size={12} />{:else}一键生成密钥对{/if}</Button
								>
							</div>
						</div>
						<div>
							<label class="mb-1 block text-[10px] text-text-muted">私钥</label>
							<textarea
								bind:value={host.sshKey}
								rows={3}
								placeholder="点击生成或粘贴"
								class="w-full resize-none rounded border border-border-secondary bg-black/30 px-2 py-1 font-mono text-[11px] text-green-400 placeholder:text-text-muted focus:border-border-focus focus:outline-none"
							></textarea>
						</div>
						<div>
							<label class="mb-1 block text-[10px] text-text-muted">公钥</label>
							<input
								type="text"
								bind:value={host.sshPubKey}
								placeholder="点击生成或粘贴"
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
								{#if copied['priv']}<Check size={10} class="text-green-400" />已复制{:else}<Copy
										size={10}
									/>私钥{/if}
							</button>
							<button
								type="button"
								class="inline-flex items-center gap-1 rounded px-2 py-1 text-[10px] text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary"
								onclick={() => onCopyText(host.sshPubKey || '', 'pub')}
								disabled={!host.sshPubKey}
							>
								{#if copied['pub']}<Check size={10} class="text-green-400" />已复制{:else}<Copy
										size={10}
									/>公钥{/if}
							</button>
							<button
								type="button"
								class="inline-flex items-center gap-1 rounded px-2 py-1 text-[10px] text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary"
								onclick={() => onCopyText(onGetCopyCmd(), 'cmd')}
								disabled={!host.sshPubKey || !host.endpoint}
							>
								{#if copied['cmd']}<Check size={10} class="text-green-400" />已复制{:else}<Terminal
										size={10}
									/>公钥设置命令{/if}
							</button>
							<button
								type="button"
								class="inline-flex items-center gap-1 rounded px-2 py-1 text-[10px] text-text-secondary transition-colors hover:bg-surface-tertiary hover:text-text-primary"
								onclick={() => onCopyText('systemctl enable --now podman.socket', 'podman')}
							>
								{#if copied['podman']}<Check
										size={10}
										class="text-green-400"
									/>已复制{:else}<Terminal size={10} />Podman命令{/if}
							</button>
						</div>
					</div>
				{/if}
				<!-- 标签 -->
				<div>
					<label class="mb-1 block text-[11px] text-text-muted">{$_t('hostModal.tags')}</label>
					<input
						type="text"
						value={(host.tags || []).join(', ')}
						onchange={handleTagsChange}
						placeholder="home, nas, prod"
						class="w-full rounded border border-border-secondary bg-surface-secondary px-3 py-1.5 text-xs text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none"
					/>
				</div>
				<!-- {$_t('hostModal.mountPoints')} -->
				<div>
					<label class="mb-2 block text-[11px] font-medium text-text-muted">{$_t('hostModal.mountPoints')}</label>
					<div class="mb-2 space-y-1">
						<div class="flex items-center gap-2 px-1 py-1">
							<input
								type="text"
								value="docker"
								readonly
								class="w-28 shrink-0 cursor-default rounded border border-border-secondary bg-surface-secondary px-2 py-1 text-[11px] text-text-secondary"
							/>
							<input
								type="text"
								value={host.mountPoints?.docker?.path || ''}
								onchange={handleDockerPathChange}
								placeholder="/var/docker"
								class="min-w-0 flex-1 rounded border border-border-secondary bg-surface-secondary px-2 py-1 font-mono text-[11px] text-text-primary focus:border-border-focus focus:outline-none"
							/>
							<span class="w-[104px] shrink-0 text-left text-[11px] font-medium text-green-400"
								>{$_t('hostModal.dockerMainDir')}</span
							>
						</div>
						{#each Object.entries(host.mountPoints || {}) as [key, mp]}
							{#if key !== 'docker'}
								<div class="flex items-center gap-2 px-1 py-1">
									<input
										type="text"
										value={key}
										readonly
										class="w-28 shrink-0 cursor-default rounded border border-border-secondary bg-surface-secondary px-2 py-1 text-[11px] text-text-secondary"
									/>
									<input
										type="text"
										value={mp.path || ''}
										onchange={(e) => handleMountPathChange(key, e)}
										placeholder="/path"
										class="min-w-0 flex-1 rounded border border-border-secondary bg-surface-secondary px-2 py-1 font-mono text-[11px] text-text-primary focus:border-border-focus focus:outline-none"
									/>
									<div class="flex w-[104px] shrink-0 items-center justify-start gap-1">
										{#if mp.readOnly}<span class="text-[10px] text-text-muted">{$_t('hostModal.readOnly')}</span>{/if}
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
							placeholder="{$_t('hostModal.name')}"
							class="w-28 shrink-0 rounded border border-border-secondary bg-surface-secondary px-2 py-1 text-[11px] text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none"
						/>
						<input
							type="text"
							bind:value={mountPath}
							placeholder="/opt/docker"
							class="min-w-0 flex-1 rounded border border-border-secondary bg-surface-secondary px-2 py-1 font-mono text-[11px] text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none"
						/>
						<div class="flex w-[104px] shrink-0 items-center gap-1">
							<label class="flex cursor-pointer items-center gap-1 text-[11px] text-text-muted"
								><input type="checkbox" bind:checked={mountReadOnly} class="rounded" /> {$_t('hostModal.readOnly')}</label
							>
							<button
								type="button"
								class="rounded bg-surface-tertiary px-2 py-1 text-[11px] text-text-primary hover:bg-surface-secondary"
								onclick={onAddMountPoint}>{$_t('hostModal.add')}</button
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
						/>{/if}测试连接
				</Button>
				<div class="flex items-center gap-2 px-1 py-1">
					<Button variant="secondary" size="sm" onclick={onClose}>{$_t('common.cancel')}</Button>
					<Button
						variant="primary"
						size="sm"
						onclick={onSave}
						disabled={!host.name || !host.endpoint}
					>
						{mode === 'add' ? tAdd() : tSave()}
					</Button>
				</div>
			</div>
		</div>
	</div>
{/if}
