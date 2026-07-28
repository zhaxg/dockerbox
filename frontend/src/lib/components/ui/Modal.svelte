<script lang="ts">
	import type { Snippet } from 'svelte';
	import { X, Maximize2, Minimize2 } from 'lucide-svelte';

	interface Props {
		open?: boolean;
		title?: string;
		size?: 'md' | 'lg' | 'xl' | 'full';
		width?: string;
		persistent?: boolean;
		draggable?: boolean;
		maximizable?: boolean;
		children: Snippet;
		headerActions?: Snippet;
		footer?: Snippet;
		onclose?: () => void;
	}

	let {
		open = $bindable(false),
		title,
		size = 'md',
		width,
		persistent = false,
		draggable = false,
		maximizable = false,
		children,
		headerActions,
		footer,
		onclose
	}: Props = $props();

	const widthClass = $derived(
		size === 'full' ? 'fixed inset-3' :
		size === 'xl' ? 'max-w-5xl' :
		size === 'lg' ? 'max-w-3xl' : 'max-w-md'
	);

	// Drag state
	let dragX = $state(0);
	let dragY = $state(0);
	let maximized = $state(false);
	let dragging = $state(false);
	let offsetX = $state(0);
	let offsetY = $state(0);

	function resetDrag() {
		dragX = 0; dragY = 0; maximized = false; dragging = false;
	}

	function onDragHeader(e: MouseEvent) {
		if (maximized || !draggable) return;
		dragging = true;
		offsetX = e.clientX - dragX;
		offsetY = e.clientY - dragY;
		e.preventDefault();
	}

	function onPointerMove(e: MouseEvent) {
		if (!dragging) return;
		dragX = e.clientX - offsetX;
		dragY = e.clientY - offsetY;
	}

	function onPointerUp() {
		dragging = false;
	}

	function toggleMaximize() {
		if (maximized) { dragX = 0; dragY = 0; maximized = false; }
		else { dragX = 0; dragY = 0; maximized = true; }
	}

	function modalStyle(): string {
		if (maximized) return '';
		return `transform: translate(${dragX}px, ${dragY}px)`;
	}

	$effect(() => {
		if (open) resetDrag();
	});

	$effect(() => {
		if (!open || !draggable) return;
		const handleMove = (e: MouseEvent) => onPointerMove(e);
		const handleUp = () => onPointerUp();
		window.addEventListener('mousemove', handleMove);
		window.addEventListener('mouseup', handleUp);
		return () => {
			window.removeEventListener('mousemove', handleMove);
			window.removeEventListener('mouseup', handleUp);
		};
	});

	function handleBackdropClick(e: MouseEvent) {
		if (e.target === e.currentTarget && !persistent) {
			onclose?.();
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape' && !persistent) {
			onclose?.();
		}
	}
</script>

{#if open}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
		role="dialog"
		aria-modal="true"
		tabindex="-1"
		onclick={handleBackdropClick}
		onkeydown={handleKeydown}
	>
		<div
			class="mx-4 flex max-h-[90vh] w-full {maximized ? 'fixed inset-3 mx-0' : widthClass} flex-col overflow-hidden rounded-lg border border-border-primary bg-surface-primary shadow-xl"
			style={modalStyle()}
		>
			{#if title}
				<div
					class="flex items-center justify-between border-b border-border-secondary px-4 py-3 {draggable ? 'cursor-move' : ''}"
					role={draggable ? 'button' : undefined}
					aria-label={draggable ? 'Drag to move' : undefined}
					onmousedown={draggable ? onDragHeader : undefined}
				>
					<h2 class="text-lg font-medium text-text-primary">{title}</h2>
					<div class="flex items-center gap-1">
						{@render headerActions?.()}
						{#if maximizable}
							<button
								type="button"
								class="rounded p-1 text-text-secondary transition-colors hover:text-text-primary"
								onclick={toggleMaximize}
								aria-label={maximized ? 'Restore' : 'Maximize'}
							>
								{#if maximized}<Minimize2 size={12} />{:else}<Maximize2 size={12} />{/if}
							</button>
						{/if}
						<button
							type="button"
							class="rounded p-1 text-text-secondary transition-colors hover:text-text-primary"
							onclick={onclose}
							aria-label="Close"
						>
							<X size={16} />
						</button>
					</div>
				</div>
			{/if}
			<div class="overflow-y-auto p-4">
				{@render children()}
			</div>
			{#if footer}
				<div class="flex justify-end gap-2 border-t border-border-secondary px-4 py-3">
					{@render footer()}
				</div>
			{/if}
		</div>
	</div>
{/if}
