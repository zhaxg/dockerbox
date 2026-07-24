import { api } from './client';

export interface HostMountPoint {
	path: string;
	readOnly: boolean;
}

export interface DockerHost {
	id: string;
	name: string;
	driver: 'tcp' | 'ssh' | 'socket';
	endpoint: string;
	sshKey?: string;
	sshPubKey?: string;
	tags?: string[];
	mountPoints?: Record<string, HostMountPoint>;
}

export interface DockerHostsConfig {
	default: string;
	hosts: Record<string, DockerHost>;
}

export interface ConnectionTestResult {
	status: 'ok' | 'error';
	message: string;
}

export const hostsApi = {
	list: () => api.get<DockerHostsConfig>('/hosts'),

	create: (host: DockerHost) => api.post<DockerHost>('/hosts', host),

	update: (id: string, host: Partial<DockerHost>) => api.put<DockerHost>(`/hosts/${id}`, host),

	delete: (id: string) => api.delete<{ message: string }>(`/hosts/${id}`),

	test: (id: string) => api.post<ConnectionTestResult>(`/hosts/${id}/test`),

	sshInstructions: () => api.get<{
		generate_key: string;
		copy_key: string;
		test_connect: string;
		public_key_path: string;
	}>('/ssh-instructions'),

	genKeyPair: () => fetch('/api/v1/hosts/sshkey', {
		method: 'POST',
		headers: { 'Authorization': 'Bearer ' + (typeof localStorage !== 'undefined' ? localStorage.getItem('accessToken') || '' : '') }
	}).then(r => r.json()),

	pushKey: (data: { user: string; host: string; port: string; password: string; pubkey: string }) =>
		fetch('/api/v1/hosts/pushkey', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + (typeof localStorage !== 'undefined' ? localStorage.getItem('accessToken') || '' : '') },
			body: JSON.stringify(data)
		}).then(r => r.json()),
};
