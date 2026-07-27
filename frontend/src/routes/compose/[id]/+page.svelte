<script lang="ts">
	import { t, getLocale } from '$lib/i18n/index.svelte';
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { dockerApi } from '$lib/api/docker';
	import { Spinner, Button } from '$lib/components/ui';
	import { ArrowLeft, Save, Play } from 'lucide-svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import type * as Monaco from 'monaco-editor';
	const tComposeLoadfailed = $derived(t("compose.loadFailed"));
	const tComposeEmptyname = $derived(t("compose.emptyName"));
	const tComposeCreatefailed = $derived(t("compose.createFailed"));
	const tComposeSavefailed = $derived(t("compose.saveFailed"));
	const tComposeNew = $derived(t("compose.new"));
	const tComposeCreate = $derived(t("compose.create"));
	const tComposeProject = $derived(t("compose.project"));
	const tComposeProjectname = $derived(t("compose.projectName"));
	const tComposeStoragepath = $derived(t("compose.storagePath"));
	const tComposeSave = $derived(t("compose.save"));
	const tComposeSaving = $derived(t("compose.saving"));
	const tComposeModified = $derived(t("compose.modified"));

	const projectId = $derived(page.params.id);
	const isNew = $derived(projectId === 'new');

	let composeContent = $state('');
	let loading = $state(true);
	let saving = $state(false);
	let projectName = $state('');
	let projectPath = $state('/vol1/1000/docker');
	let error = $state('');
	let containerElement: HTMLDivElement | null = $state(null);
	let editor: Monaco.editor.IStandaloneCodeEditor | null = $state(null);
	let monaco: typeof Monaco | null = $state(null);
	let dirty = $state(false);

	const canSave = $derived(dirty && !saving && !loading);

	onMount(async () => {
		if (!isNew) {
			await loadComposeFile();
		} else {
			composeContent = "services:\n  my-service:\n    image: nginx:latest\n    ports:\n      - \"8080:80\"\n";
			loading = false;
		}
		await loadMonaco();
	});

	async function loadComposeFile() {
		loading = true;
		try {
			const data = await dockerApi.get<{ content: string }>(`/docker/compose/${projectId}/file`);
			composeContent = data?.content || '';
		} catch (e) {
			error = e instanceof Error ? e.message : tComposeLoadfailed;
		} finally {
			loading = false;
		}
	}

	async function loadMonaco() {
		if (typeof window === 'undefined') return;
		try {
			const monacoModule = await import('monaco-editor');
			monaco = monacoModule.default || monacoModule;
			if (containerElement && !editor) {
				createEditor();
			}
		} catch (e) {
			console.error('Failed to load Monaco:', e);
		}
	}

	function createEditor() {
		if (!monaco || !containerElement || editor) return;

		monaco.editor.defineTheme('dockerbox-dark', {
			base: 'vs-dark',
			inherit: true,
			rules: [],
			colors: {
				'editor.background': '#1e1e1e',
				'editor.foreground': '#d4d4d4',
				'editorLineNumber.foreground': '#5a5a5a',
				'editorLineNumber.activeForeground': '#c6c6c6',
				'editor.selectionBackground': '#264f78',
				'editor.lineHighlightBackground': '#2a2a2a'
			}
		});

		editor = monaco.editor.create(containerElement, {
			value: composeContent,
			language: 'yaml',
			theme: 'dockerbox-dark',
			minimap: { enabled: false },
			fontSize: 13,
			lineNumbers: 'on',
			scrollBeyondLastLine: false,
			automaticLayout: true,
			tabSize: 2,
			wordWrap: 'on'
		});

		editor.onDidChangeModelContent(() => {
			composeContent = editor?.getValue() || '';
			dirty = true;
		});
	}

	$effect(() => {
		if (containerElement && monaco && !editor) {
			createEditor();
		}
	});

	$effect(() => {
		if (editor && !loading) {
			editor.setValue(composeContent);
			dirty = false;
		}
	});

	async function handleSave() {
		if (isNew) {
			// Create new project
			if (!projectName.trim()) { error = tComposeEmptyname; return; }
			saving = true; error = '';
			try {
				await dockerApi.post('/docker/compose', {
					name: projectName.trim(),
					composeContent,
					basePath: projectPath
				});
				goto(resolve('/compose'));
			} catch (e) {
				error = e instanceof Error ? e.message : tComposeCreatefailed;
			} finally {
				saving = false;
			}
		} else {
			// Update existing project
			saving = true; error = '';
			try {
				await dockerApi.put(`/docker/compose/${projectId}/file`, { content: composeContent });
				dirty = false;
			} catch (e) {
				error = e instanceof Error ? e.message : tComposeSavefailed;
			} finally {
				saving = false;
			}
		}
	}
</script>

<div class="flex h-full flex-col bg-surface-primary">
	<!-- Header -->
	<div class="flex items-center justify-between border-b border-border-secondary px-4 py-3">
		<div class="flex items-center gap-3">
			<button type="button" class="text-text-muted hover:text-text-primary" onclick={() => goto(resolve('/compose'))}>
				<ArrowLeft size={18} />
			</button>
			<h1 class="text-base font-semibold text-text-primary">
				{isNew ? tComposeNew + ' Compose ' + tComposeProject : projectId}
			</h1>
			{#if dirty}
				<span class="text-[11px] text-orange-400">● {tComposeModified}</span>
			{/if}
		</div>
		<div class="flex items-center gap-2">
			{#if isNew}
				<input type="text" bind:value={projectName} placeholder={tComposeProjectname}
					class="h-7 w-40 rounded border border-border-secondary bg-surface-secondary px-2 text-xs text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none" />
				<input type="text" bind:value={projectPath} placeholder={tComposeStoragepath}
					class="h-7 w-48 rounded border border-border-secondary bg-surface-secondary px-2 text-xs text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none" />
			{/if}
			{#if error}
				<span class="text-xs text-red-400">{error}</span>
			{/if}
			<Button variant="primary" size="sm" onclick={handleSave} disabled={!canSave}>
				{#if saving}
					<Spinner size={14} class="mr-1" /> {tComposeSaving}...
				{:else}
					<Save size={14} class="mr-1" /> {isNew ? tComposeCreate : tComposeSave}
				{/if}
			</Button>
		</div>
	</div>

	<!-- Editor -->
	<div class="flex-1 overflow-hidden">
		{#if loading}
			<div class="flex items-center justify-center py-12"><Spinner size="lg" /></div>
		{:else}
			<div bind:this={containerElement} class="h-full w-full"></div>
		{/if}
	</div>
</div>
