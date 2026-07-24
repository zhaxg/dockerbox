<script lang="ts">
	/**
	 * UploadProgress component — compact floating panel
	 */

	import type { UploadProgress as UploadProgressType } from '$lib/utils/upload';
	import { formatFileSize, formatPercentage } from '$lib/utils/format';
	import { X, Upload, Check, AlertCircle } from 'lucide-svelte';
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

	const activeCount = $derived(
		uploads.filter((u) => u.status === 'pending' || u.status === 'uploading').length
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
	<div class="w-[340px] overflow-hidden rounded-lg border border-border-secondary bg-surface-primary/95 shadow-xl backdrop-blur">
		<!-- Header -->
		<div class="flex items-center justify-between border-b border-border-secondary px-3 py-2">
			<div class="flex items-center gap-2">
				<Upload size={14} class="text-text-secondary" />
				<span class="text-xs font-medium text-text-primary">
					上传 {activeCount > 0 ? `(${activeCount})` : ''}
				</span>
			</div>
			{#if uploads.some(u => isTerminal(u.status)) && onClearCompleted}
				<button type="button" class="rounded px-1.5 py-0.5 text-[10px] text-text-muted hover:bg-surface-secondary hover:text-text-primary transition-colors" onclick={onClearCompleted}>
					清除
				</button>
			{/if}
		</div>

		<!-- Upload list -->
		<ul class="m-0 max-h-[240px] list-none overflow-y-auto p-0" role="list">
			{#each filteredUploads as upload (upload.uploadId)}
				<li class="border-b border-border-secondary/50 px-3 py-2 last:border-b-0 hover:bg-surface-secondary/50 transition-colors">
					<!-- File name row -->
					<div class="flex items-center gap-2 mb-1.5">
						{#if upload.status === 'complete'}
							<Check size={12} class="shrink-0 text-emerald-400" />
						{:else if upload.status === 'error'}
							<AlertCircle size={12} class="shrink-0 text-red-400" />
						{:else}
							<button
								type="button"
								class="group flex h-4 w-4 shrink-0 items-center justify-center rounded-sm border border-border-secondary bg-transparent text-text-muted cursor-pointer hover:border-red-400 hover:text-red-400 transition-colors"
								onclick={() => isTerminal(upload.status) ? onRemove?.(upload.uploadId) : onCancel?.(upload.uploadId)}
							>
								<X size={10} />
							</button>
						{/if}
						<span class="flex-1 overflow-hidden text-[12px] font-medium text-ellipsis whitespace-nowrap text-text-primary" title={upload.fileName}>
							{upload.fileName}
						</span>
						<span class="shrink-0 text-[10px] text-text-muted">
							{formatFileSize(upload.totalSize)}
						</span>
					</div>

					<!-- Progress bar + status -->
					<div class="flex items-center gap-2">
						<div class="flex-1">
							<ProgressBar
								value={upload.percentage}
								size="sm"
								variant={getProgressVariant(upload.status)}
							/>
						</div>
						<span class="shrink-0 w-10 text-right text-[10px] font-medium {getStatusColor(upload.status)}">
							{getStatusText(upload)}
						</span>
					</div>
				</li>
			{/each}
		</ul>
	</div>
{/if}
