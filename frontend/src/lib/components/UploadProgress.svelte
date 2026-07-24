<script lang="ts">
	/**
	 * UploadProgress — flat list, no card wrapper
	 */

	import type { UploadProgress as UploadProgressType } from '$lib/utils/upload';
	import { formatFileSize, formatPercentage } from '$lib/utils/format';
	import { X, Check, AlertCircle } from 'lucide-svelte';
	import { ProgressBar } from '$lib/components/ui';

	interface Props {
		uploads: UploadProgressType[];
		onCancel?: (uploadId: string) => void;
		onRemove?: (uploadId: string) => void;
		onClearCompleted?: () => void;
		showCompleted?: boolean;
	}

	let {
		uploads = [],
		onCancel,
		onRemove,
		onClearCompleted,
		showCompleted = true
	}: Props = $props();

	const filteredUploads = $derived(
		showCompleted
			? uploads
			: uploads.filter((u) => u.status !== 'complete' && u.status !== 'cancelled')
	);

	function isTerminal(status: UploadProgressType['status']): boolean {
		return status === 'complete' || status === 'error' || status === 'cancelled';
	}

	function getStatusColor(status: UploadProgressType['status']): string {
		switch (status) {
			case 'complete': return 'text-emerald-400';
			case 'error': return 'text-red-400';
			case 'uploading': return 'text-blue-400';
			default: return 'text-text-muted';
		}
	}

	function getStatusText(upload: UploadProgressType): string {
		switch (upload.status) {
			case 'pending': return '排队中';
			case 'uploading': return formatPercentage(upload.percentage, 0, false);
			case 'complete': return '完成';
			case 'error': return upload.error || '失败';
			case 'cancelled': return '已取消';
			default: return '';
		}
	}

	function getProgressVariant(status: UploadProgressType['status']): 'default' | 'success' | 'warning' | 'danger' {
		switch (status) {
			case 'complete': return 'success';
			case 'error': return 'danger';
			default: return 'default';
		}
	}
</script>

{#if filteredUploads.length > 0}
	<div class="divide-y divide-border-secondary/50">
		{#each filteredUploads as upload (upload.uploadId)}
			<div class="flex items-center gap-2 px-3 py-2">
				<!-- Status icon -->
				{#if upload.status === 'complete'}
					<Check size={12} class="shrink-0 text-emerald-400" />
				{:else if upload.status === 'error'}
					<AlertCircle size={12} class="shrink-0 text-red-400" />
				{:else}
					<button
						type="button"
						class="flex h-4 w-4 shrink-0 items-center justify-center rounded-sm border border-border-secondary bg-transparent text-text-muted cursor-pointer hover:border-red-400 hover:text-red-400 transition-colors"
						onclick={() => isTerminal(upload.status) ? onRemove?.(upload.uploadId) : onCancel?.(upload.uploadId)}
					>
						<X size={10} />
					</button>
				{/if}

				<!-- Filename + size -->
				<div class="flex flex-1 min-w-0 items-center gap-2">
					<span class="overflow-hidden text-[12px] font-medium text-ellipsis whitespace-nowrap text-text-primary" title={upload.fileName}>
						{upload.fileName}
					</span>
					<span class="shrink-0 text-[10px] text-text-muted">
						{formatFileSize(upload.totalSize)}
					</span>
				</div>

				<!-- Progress + status text -->
				<div class="flex w-24 items-center gap-1.5">
					<ProgressBar value={upload.percentage} size="sm" variant={getProgressVariant(upload.status)} />
					<span class="shrink-0 w-9 text-right text-[10px] font-medium {getStatusColor(upload.status)}">
						{getStatusText(upload)}
					</span>
				</div>
			</div>
		{/each}
	</div>
{/if}
