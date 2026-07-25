/**
 * Docker API helper - provides hostId-aware Docker API methods.
 * Each call explicitly passes hostId as a query parameter.
 */
import { apiRequest } from './client';

function getHostId(): string {
	if (typeof window === 'undefined') return '';
	return localStorage.getItem('currentHostId') || '';
}

export const dockerApi = {
	get: <T>(endpoint: string, params?: Record<string, string | number | boolean | undefined>) =>
		apiRequest<T>(endpoint, { method: 'GET', params, hostId: getHostId() }),

	post: <T>(endpoint: string, body?: unknown) =>
		apiRequest<T>(endpoint, { method: 'POST', body, hostId: getHostId() }),

	put: <T>(endpoint: string, body?: unknown) =>
		apiRequest<T>(endpoint, { method: 'PUT', body, hostId: getHostId() }),

	delete: <T>(endpoint: string, params?: Record<string, string | number | boolean | undefined>) =>
		apiRequest<T>(endpoint, { method: 'DELETE', params, hostId: getHostId() })
};
