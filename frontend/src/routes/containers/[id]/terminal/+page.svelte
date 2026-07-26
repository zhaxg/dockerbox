<script lang="ts">
	import { t, getLocale } from '$lib/i18n/index.svelte';
	import { page } from '$app/state';
	import { onMount, onDestroy } from 'svelte';
	import { Button } from '$lib/components/ui';
	import { ArrowLeft, Terminal } from 'lucide-svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';

	const containerId = $derived(page.params.id);

	const tContainersConnectedtoterminal = $derived(t('containers.connectedToTerminal'));
	const tContainersConnectionlost = $derived(t('containers.connectionLost'));
	const tContainersConnectionerror = $derived(t('containers.connectionError'));
	const tContainersBack = $derived(t('containers.back'));
	const tContainersTerminal = $derived(t('containers.terminal'));
	const tContainersConnected = $derived(t('containers.connected'));
	const tContainersDisconnected = $derived(t('containers.disconnected'));

	let terminalEl = $state<HTMLDivElement | null>(null);
	let ws = $state<WebSocket | null>(null);
	let outputBuffer = $state('');
	let connected = $state(false);

	onMount(() => {
		connectWebSocket();
	});

	onDestroy(() => {
		ws?.close();
	});

	function connectWebSocket() {
		const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		const host = window.location.host;
		const token = localStorage.getItem('accessToken') || '';
		ws = new WebSocket(`${protocol}//${host}/api/v1/docker/containers/${containerId}/exec`);

		ws.onopen = () => {
			connected = true;
			outputBuffer = tContainersConnectedtoterminal + '...\r\n';
			ws?.send(JSON.stringify({ type: 'auth', token }));
		};

		ws.onmessage = (event) => {
			let text: string;
			if (typeof event.data === 'string') {
				text = event.data;
			} else if (event.data instanceof ArrayBuffer) {
				// Docker exec with Tty=true sends raw output, no multiplexed header
				text = new TextDecoder().decode(event.data);
			} else if (event.data instanceof Blob) {
				// Blob needs async read
				event.data.arrayBuffer().then((buf) => {
					outputBuffer += new TextDecoder().decode(buf);
					scrollToBottom();
				});
				return;
			} else {
				text = String(event.data);
			}
			outputBuffer += text;
			scrollToBottom();
		};

		ws.onclose = () => {
			connected = false;
			outputBuffer += '\r\n' + tContainersConnectionlost;
		};

		ws.onerror = () => {
			connected = false;
			outputBuffer += '\r\n' + tContainersConnectionerror;
		};
	}

	function scrollToBottom() {
		if (terminalEl) {
			terminalEl.scrollTop = terminalEl.scrollHeight;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		// Prevent default for all keys when terminal is focused
		if (e.metaKey || e.ctrlKey) {
			// Allow Ctrl+C (ETX = \x03), Ctrl+D (EOT = \x04), etc.
			if (e.key === 'c') sendRaw('\x03');
			else if (e.key === 'd') sendRaw('\x04');
			else if (e.key === 'z') sendRaw('\x1a');
			else if (e.key === 'l') sendRaw('\x0c');
			e.preventDefault();
			return;
		}

		if (e.key === 'Enter') {
			sendRaw('\r');
		} else if (e.key === 'Backspace') {
			sendRaw('\x7f');
		} else if (e.key === 'Tab') {
			sendRaw('\t');
			e.preventDefault();
		} else if (e.key === 'ArrowUp') {
			sendRaw('\x1b[A');
			e.preventDefault();
		} else if (e.key === 'ArrowDown') {
			sendRaw('\x1b[B');
			e.preventDefault();
		} else if (e.key === 'ArrowRight') {
			sendRaw('\x1b[C');
			e.preventDefault();
		} else if (e.key === 'ArrowLeft') {
			sendRaw('\x1b[D');
			e.preventDefault();
		} else if (e.key === 'Home') {
			sendRaw('\x1b[H');
			e.preventDefault();
		} else if (e.key === 'End') {
			sendRaw('\x1b[F');
			e.preventDefault();
		} else if (e.key.length === 1 && !e.isComposing) {
			sendRaw(e.key);
		}
		e.preventDefault();
	}

	function sendRaw(data: string) {
		if (ws?.readyState === WebSocket.OPEN) {
			ws.send(new TextEncoder().encode(data));
		}
	}
</script>

<div class="flex h-full flex-col p-6">
	<!-- Header -->
	<div class="mb-4 flex items-center gap-4">
		<Button variant="secondary" onclick={() => goto(resolve(`/containers/${containerId}`))}>
			<ArrowLeft size={16} class="mr-2" />
			{tContainersBack}
		</Button>
		<h1 class="text-2xl font-semibold text-text-primary">
			<Terminal size={20} class="mr-2" />
			{tContainersTerminal} - {containerId}
		</h1>
		<span class="text-sm text-text-secondary">
			{connected ? tContainersConnected : tContainersDisconnected}
		</span>
	</div>

	<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		bind:this={terminalEl}
		class="flex-1 overflow-auto rounded bg-black p-4 font-mono text-sm text-green-400 focus:outline-none"
		tabindex="0"
		onkeydown={handleKeydown}
	>
		<pre class="whitespace-pre-wrap">{outputBuffer}</pre>
	</div>
</div>
