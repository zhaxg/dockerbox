/**
 * File Context Menu Configuration
 * Centralized definition of context menu items for file operations
 */

import type { ContextMenuItem } from '$lib/components/ui/ContextMenu.svelte';
import type { FileInfo } from '$lib/api/files';
import { t, getLocale } from '$lib/i18n/index.svelte';
import {
	Copy,
	Scissors,
	ClipboardPaste,
	FilePlus,
	FolderPlus,
	Pencil,
	Trash2,
	Download,
	Info,
	Pin,
	PinOff,
	RefreshCw,
	FileText
} from 'lucide-svelte';

export type FileContextAction =
	| 'new-file'
	| 'new-folder'
	| 'copy'
	| 'cut'
	| 'paste'
	| 'pin'
	| 'unpin'
	| 'rename'
	| 'delete'
	| 'download'
	| 'properties'
	| 'refresh'
	| 'open-with-notepad';

export interface FileContextMenuOptions {
	/** Selected items for the context menu */
	items: FileInfo[];
	/** Whether paste is available (clipboard has items) */
	canPaste: boolean;
	/** Paths currently pinned in the sidebar favorites section */
	favoritePaths?: Set<string>;
	/** Whether new files/folders can be created in the current location */
	canCreate?: boolean;
	/** Whether creation actions should be included in this menu */
	includeCreateActions?: boolean;
}

/**
 * Get context menu items for file operations
 * Configures disabled states based on selection
 */
/** Max file size for text viewer: 2MB */
const MAX_NOTEPAD_SIZE = 2 * 1024 * 1024;

function canOpenAsText(file: FileInfo): boolean {
	return !file.isDir && file.size <= MAX_NOTEPAD_SIZE;
}

export function getFileContextMenuItems(options: FileContextMenuOptions): ContextMenuItem[] {
	const {
		items,
		canPaste,
		favoritePaths = new Set<string>(),
		canCreate = false,
		includeCreateActions = true
	} = options;
	const hasSelection = items.length > 0;
	const hasMultiple = items.length > 1;
	const hasFolder = items.some((i) => i.isDir);
	const singleFolder = !hasMultiple && items[0]?.isDir ? items[0] : null;
	const singleFile = !hasMultiple && items.length === 1 && !items[0].isDir ? items[0] : null;
	const isFavorite = singleFolder ? favoritePaths.has(singleFolder.path) : false;
	const createItems: ContextMenuItem[] = includeCreateActions
		? [
				{ id: 'new-file', label: t('files.newFile'), icon: FilePlus, disabled: !canCreate },
				{ id: 'new-folder', label: t('files.newFolder'), icon: FolderPlus, disabled: !canCreate },
				{ id: 'separator-create', label: '', separator: true }
			]
		: [];

	if (!hasSelection) {
		return [
			...createItems,
			{ id: 'paste', label: t('files.paste'), icon: ClipboardPaste, shortcut: 'Ctrl+V', disabled: !canPaste },
			{ id: 'separator-refresh', label: '', separator: true },
			{ id: 'refresh', label: t('files.refresh'), icon: RefreshCw, shortcut: 'F5' }
		];
	}

	return [
		...(singleFile && canOpenAsText(items[0])
			? [{ id: 'open-with-notepad', label: t('files.preview'), icon: FileText }]
			: []),
		{ id: 'copy', label: t('files.copy'), icon: Copy, shortcut: 'Ctrl+C' },
		{ id: 'cut', label: t('files.cut'), icon: Scissors, shortcut: 'Ctrl+X' },
		{ id: 'paste', label: t('files.paste'), icon: ClipboardPaste, shortcut: 'Ctrl+V', disabled: !canPaste },
		...(singleFolder
			? [
					{
						id: isFavorite ? 'unpin' : 'pin',
						label: isFavorite ? t('files.unpin') : t('files.pin'),
						icon: isFavorite ? PinOff : Pin
					}
				]
			: []),
		{ id: 'separator-1', label: '', separator: true },
		{ id: 'rename', label: t('files.rename'), icon: Pencil, shortcut: 'F2', disabled: hasMultiple },
		{ id: 'delete', label: t('files.delete'), icon: Trash2, shortcut: 'Del' },
		{ id: 'separator-2', label: '', separator: true },
		{ id: 'download', label: t('files.download'), icon: Download, disabled: hasFolder },
		{ id: 'properties', label: t('files.properties'), icon: Info, disabled: hasMultiple },
		{ id: 'separator-refresh', label: '', separator: true },
		{ id: 'refresh', label: t('files.refresh'), icon: RefreshCw, shortcut: 'F5' }
	];
}
