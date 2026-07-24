<script lang="ts">
	/**
	 * Main page - redirects to overview
	 */
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { Spinner } from '$lib/components/ui';

	onMount(() => {
		goto('/overview', { replaceState: true });
	});
</script>

<div class="flex min-h-screen items-center justify-center bg-surface-primary">
	<div class="flex flex-col items-center gap-4 text-text-secondary">
		<Spinner />
		<p>Redirecting to overview...</p>
	</div>
</div>
