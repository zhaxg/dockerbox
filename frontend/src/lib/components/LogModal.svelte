<script lang="ts">
	import { t, getLocale } from '$lib/i18n/index.svelte';
	import { X, Maximize2, Minimize2 } from 'lucide-svelte';
	const tComposeAbort = $derived(t("compose.abort"));

	let { open = false, name = '', content = '', loading = false, streaming = false, onClose, onAbort }: {
		open: boolean;
		name: string;
		content: string;
		loading: boolean;
		streaming: boolean;
		onClose: () => void;
		onAbort?: () => void;
	} = $props();

	let contentEl: HTMLDivElement | null = $state(null);

	// Drag + maximize state
	let dragX = $state(0);
	let dragY = $state(0);
	let maximized = $state(false);
	let dragging = $state(false);
	let offsetX = $state(0);
	let offsetY = $state(0);

	function resetDrag() { dragX = 0; dragY = 0; maximized = false; dragging = false; }

	function onDragHeader(e: MouseEvent) {
		if (maximized) return;
		dragging = true;
		offsetX = e.clientX - dragX;
		offsetY = e.clientY - dragY;
		e.preventDefault();
	}

	function onDragMove(e: MouseEvent) {
		if (!dragging) return;
		dragX = e.clientX - offsetX;
		dragY = e.clientY - offsetY;
	}

	function onDragEnd() { dragging = false; }

	function toggleMaximize() {
		if (maximized) { dragX = 0; dragY = 0; maximized = false; }
		else { dragX = 0; dragY = 0; maximized = true; }
	}

	function modalStyle(): string {
		if (maximized) return '';
		return `transform: translate(${dragX}px, ${dragY}px)`;
	}

	$effect(() => {
		if (contentEl && content) {
			contentEl.scrollTop = contentEl.scrollHeight;
		}
	});

	$effect(() => {
		if (open) resetDrag();
	});

	$effect(() => {
		if (!open) return;
		const handleMove = (e: MouseEvent) => onDragMove(e);
		const handleUp = () => onDragEnd();
		window.addEventListener('mousemove', handleMove);
		window.addEventListener('mouseup', handleUp);
		return () => {
			window.removeEventListener('mousemove', handleMove);
			window.removeEventListener('mouseup', handleUp);
		};
	});
</script>

{#if open}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
		<div class="flex flex-col rounded-lg bg-surface-primary p-3 shadow-xl border border-border-secondary {maximized ? 'fixed inset-3' : 'h-[70vh] w-[700px]'}" style={modalStyle()}>
			<div class="flex items-center justify-between px-3 py-2 cursor-move" role="button" tabindex="-1" onmousedown={onDragHeader}>
				<h3 class="text-sm font-semibold text-text-primary">{name}</h3>
				<div class="flex items-center gap-1">
					<button type="button" class="rounded p-1 text-text-secondary transition-colors hover:text-text-primary" onclick={toggleMaximize}>
						{#if maximized}<Minimize2 size={12} />{:else}<Maximize2 size={12} />{/if}
					</button>
					<button type="button" class="rounded p-1 text-text-secondary transition-colors hover:text-text-primary" onclick={onClose}><X size={16} /></button>
				</div>
			</div>
			<div bind:this={contentEl} class="flex-1 overflow-auto rounded-md bg-black p-3">
				<pre class="whitespace-pre font-mono text-xs overflow-x-auto" style="color: #00ff00">{content}</pre>
			</div>
			{#if loading && onAbort}
				<div class="flex justify-end border-t border-border-secondary px-4 py-3">
					<button type="button" class="inline-flex items-center gap-1 rounded bg-red-500/15 px-3 py-1 text-xs text-red-400 hover:bg-red-500/25 transition-colors" onclick={onAbort}>{tComposeAbort}</button>
				</div>
			{/if}
		</div>
	</div>
{/if}
