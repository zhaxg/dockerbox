<script lang="ts">
	import { Spinner } from '$lib/components/ui';
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
		<div class="flex h-[70vh] w-[700px] flex-col rounded-lg bg-surface-primary shadow-xl border border-border-secondary">
			<div class="flex items-center justify-between border-b border-border-secondary px-4 py-3">
				<div class="flex items-center gap-2">
					<h3 class="text-sm font-semibold text-text-primary">{name}</h3>
					{#if streaming}
						<span class="inline-flex items-center gap-1 rounded bg-green-500/15 px-2 py-0.5 text-[11px] text-green-400">
							<span class="h-1.5 w-1.5 rounded-full bg-green-400 animate-pulse"></span> 实时
						</span>
					{:else if loading}
						<Spinner size={14} />
					{/if}
				</div>
				<button type="button" class="text-text-muted hover:text-text-primary" onclick={onClose}><X size={16} /></button>
			</div>
			<div bind:this={contentEl} class="flex-1 overflow-auto overflow-x-auto p-4">
				<pre class="whitespace-pre font-mono text-xs text-green-400">{content}</pre>
			</div>
			{#if loading && onAbort}
				<div class="flex justify-end border-t border-border-secondary px-4 py-3">
					<button type="button" class="inline-flex items-center gap-1 rounded bg-red-500/15 px-3 py-1 text-xs text-red-400 hover:bg-red-500/25 transition-colors" onclick={onAbort}>终止</button>
				</div>
			{/if}
		</div>
	</div>
{/if}
