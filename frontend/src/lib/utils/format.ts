/**
 * Formatting utilities for file sizes and dates
 * Requirements: 1.1
 */

/**
 * File size units for formatting
 */
const SIZE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'] as const;

/**
 * Format a file size in bytes to a human-readable string
 * @param bytes - The size in bytes
 * @param decimals - Number of decimal places (default: 2)
 * @returns Formatted string like "1.5 MB"
 */
export function formatFileSize(bytes: number, decimals: number = 2): string {
	if (bytes === 0) return '0 B';
	if (bytes < 0) return 'Invalid size';
	if (!Number.isFinite(bytes)) return 'Invalid size';

	const k = 1024;
	const dm = Math.max(0, decimals);
	const i = Math.floor(Math.log(bytes) / Math.log(k));
	const unitIndex = Math.min(i, SIZE_UNITS.length - 1);

	const value = bytes / Math.pow(k, unitIndex);

	// Remove trailing zeros for cleaner display
	const formatted = value.toFixed(dm);
	const trimmed = parseFloat(formatted).toString();

	return `${trimmed} ${SIZE_UNITS[unitIndex]}`;
}

/**
 * Format a date for display in file listings (compact format)
 * @param date - Date object, ISO string, or timestamp
 * @returns Compact date string
 */
export function formatFileDate(date: Date | string | number): string {
	const dateObj = date instanceof Date ? date : new Date(date);

	if (isNaN(dateObj.getTime())) {
		return '-';
	}

	const now = new Date();
	const isToday = dateObj.toDateString() === now.toDateString();
	const isThisYear = dateObj.getFullYear() === now.getFullYear();

	if (isToday) {
		return dateObj.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' });
	}

	if (isThisYear) {
		return dateObj.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
	}

	return dateObj.toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' });
}

/**
 * Format a percentage value
 * @param value - Value between 0 and 1, or 0 and 100
 * @param decimals - Number of decimal places (default: 0)
 * @param assumeDecimal - If true, assumes value is 0-1 range (default: auto-detect)
 * @returns Formatted percentage string like "75%"
 */
export function formatPercentage(
	value: number,
	decimals: number = 0,
	assumeDecimal?: boolean
): string {
	if (!Number.isFinite(value)) return 'Invalid';

	// Auto-detect if value is in 0-1 or 0-100 range
	const isDecimal = assumeDecimal ?? (value >= 0 && value <= 1);
	const percentage = isDecimal ? value * 100 : value;

	return `${percentage.toFixed(decimals)}%`;
}

// Note: getFileTypeDescription has been moved to $lib/utils/fileTypes.ts
// Import it from there: import { getFileTypeDescription } from '$lib/utils/fileTypes';
export { getFileTypeDescription } from '$lib/utils/fileTypes';
