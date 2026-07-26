<script lang="ts">
	import { X } from 'lucide-svelte';

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

	$effect(() => {
		if (contentEl && content) {
			contentEl.scrollTop = contentEl.scrollHeight;
		}
	});
</script>

{#if open}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
		<div class="flex h-[70vh] w-[700px] flex-col rounded-lg bg-surface-primary p-3 shadow-xl border border-border-secondary">
			<div class="flex items-center justify-between px-3 py-2">
				<div class="flex items-center gap-2">
					<h3 class="text-sm font-semibold text-text-primary">{name}</h3>
				</div>
				<button type="button" class="text-text-muted hover:text-text-primary" onclick={onClose}><X size={16} /></button>
			</div>
			<div bind:this={contentEl} class="flex-1 overflow-auto rounded-md bg-black p-3">
				<pre class="whitespace-pre font-mono text-xs overflow-x-auto" style="color: #00ff00">{content}</pre>
			</div>
			{#if loading && onAbort}
				<div class="flex justify-end border-t border-border-secondary px-4 py-3">
					<button type="button" class="inline-flex items-center gap-1 rounded bg-red-500/15 px-3 py-1 text-xs text-red-400 hover:bg-red-500/25 transition-colors" onclick={onAbort}>终止</button>
				</div>
			{/if}
		</div>
	</div>
{/if}
