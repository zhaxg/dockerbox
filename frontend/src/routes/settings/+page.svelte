<script lang="ts">
	/**
	 * Settings page - workspace-style preferences screen matching the file browser shell.
	 */
	import { onDestroy, tick } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { authStore } from '$lib/stores/auth';
	import {
		DEFAULT_ACCENT_COLOR,
		isValidBackgroundImage,
		isValidAccentColor,
		normalizeBackgroundImage,
		normalizeAccentColor,
		FONT_LIST,
		applyAccentColor,
		applyFonts,
		applyTheme,
		settingsStore,
		type UserSettings
	} from '$lib/stores/settings';
	import SearchBar from '$lib/components/SearchBar.svelte';
	import WallpaperSettings from '$lib/components/settings/wallpaper/WallpaperSettings.svelte';
	import { Button, ProgressButton, Select, Toggle } from '$lib/components/ui';
	import { normalizeBackgroundImageMode } from '$lib/utils/wallpaper';
	import { t, setLocale, getLocale } from '$lib/i18n/index.svelte';
const tSettingsSystemdefault = $derived(t("settings.systemDefault"));
const tSettingsSortbytype = $derived(t("settings.sortByType"));
const tSettingsSortbysize = $derived(t("settings.sortBySize"));
const tSettingsSortbyname = $derived(t("settings.sortByName"));
const tSettingsSortbydate = $derived(t("settings.sortByDate"));
const tSettingsListview = $derived(t("settings.listView"));
const tSettingsGridview = $derived(t("settings.gridView"));
const tSettingsDescending = $derived(t("settings.descending"));
const tSettingsAscending = $derived(t("settings.ascending"));
const tSettingsShowall = $derived(t("settings.showAll"));
const tSettingsAccentcolor = $derived(t("settings.accentColor"));
const tSettingsCompactmode = $derived(t("settings.compactMode"));
const tSettingsConfirmbeforedelete = $derived(t("settings.confirmBeforeDelete"));
const tSettingsDefaultview = $derived(t("settings.defaultView"));
const tSettingsFiledisplay = $derived(t("settings.fileDisplay"));
const tSettingsPreviewonsingleclick = $derived(t("settings.previewOnSingleClick"));
const tSettingsResetdefaults = $derived(t("settings.resetDefaults"));
const tSettingsSearchsettings = $derived(t("settings.searchSettings"));
const tSettingsShowfileextensions = $derived(t("settings.showFileExtensions"));
const tSettingsShowhiddenfiles = $derived(t("settings.showHiddenFiles"));
const tSettingsSignedinsession = $derived(t("settings.signedInSession"));
const tSettingsSortby = $derived(t("settings.sortBy"));
const tSettingsSortdirection = $derived(t("settings.sortDirection"));
const tSettingsViewmode = $derived(t("settings.viewMode"));
const tCommonCancel = $derived(t("common.cancel"));
const tCommonSave = $derived(t("common.save"));
const tCommonSaved = $derived(t("common.saved"));
const tCommonSaving = $derived(t("common.saving"));
const tSettingsAccount = $derived(t("settings.account"));
const tSettingsBehavior = $derived(t("settings.behavior"));
const tSettingsChinese = $derived(t("settings.chinese"));
const tSettingsDefault = $derived(t("settings.default"));
const tSettingsEnglish = $derived(t("settings.english"));
const tSettingsFont = $derived(t("settings.font"));
const tSettingsLanguage = $derived(t("settings.language"));
const tSettingsLogout = $derived(t("settings.logout"));
const tSettingsPersonalization = $derived(t("settings.personalization"));
const tSettingsPreferences = $derived(t("settings.preferences"));
const tSettingsTheme = $derived(t("settings.theme"));
const tSettingsDark = $derived(t("settings.dark"));
const tSettingsLight = $derived(t("settings.light"));
const tSettingsSystem = $derived(t("settings.system"));
const tSettingsTitle = $derived(t("settings.title"));
		import {
		deleteLocalWallpaper,
		isInlineWallpaperDataUrl,
		isLocalWallpaperReference,
		saveLocalWallpaperDataUrl
	} from '$lib/utils/wallpaperStorage';
	import {
		ChevronLeft,
		Eye,
		Globe,
		Layout,
		LogOut,
		Moon,
		MousePointer,
		Palette,
		RotateCcw,
		Save,
		Settings,
		Sun,
		User,
		PaintRollerIcon,
		X
	} from 'lucide-svelte';

	type SettingsSectionId = 'display' | 'personalization' | 'behavior' | 'defaults' | 'account';
	type SettingsCategory = 'all' | SettingsSectionId;
	type ApplyProgressVariant = 'default' | 'success' | 'danger';

	let settings = $state<UserSettings>({ ...$settingsStore });
	
	

	function handleFontChange(e: Event) {
		settings.uiFont = (e.target as HTMLSelectElement).value;
		applyFonts(settings.uiFont);
	}
	let activeCategory = $state<SettingsCategory>('all');
	let searchQuery = $state('');
	let isApplyingSettings = $state(false);
	let applyProgress = $state(0);
	let applyProgressStatus = $state('');
	let applyProgressVariant = $state<ApplyProgressVariant>('default');
	let applyProgressResetTimer: ReturnType<typeof setTimeout> | null = null;

	const hasChanges = $derived(JSON.stringify(settings) !== JSON.stringify($settingsStore));
	const normalizedSearch = $derived(searchQuery.trim().toLowerCase());
	const accentColorIsValid = $derived(isValidAccentColor(settings.accentColor));
	const accentColorValue = $derived(
		normalizeAccentColor(settings.accentColor) ?? DEFAULT_ACCENT_COLOR
	);
	const backgroundImageIsValid = $derived(isValidBackgroundImage(settings.backgroundImage));
	const canSave = $derived(
		hasChanges && accentColorIsValid && backgroundImageIsValid && !isApplyingSettings
	);
	const showApplyProgress = $derived(isApplyingSettings || applyProgress > 0);
	const saveButtonText = $derived.by(() => {
		if (applyProgressVariant === 'danger' && applyProgress > 0) return 'Failed';
		if (applyProgressVariant === 'success' && applyProgress >= 100) return tCommonSaved;
		return isApplyingSettings ? tCommonSaving : tCommonSave;
	});

	const navItems = $derived<Array<{
		id: SettingsCategory;
		label: string;
		icon: typeof Eye;
	}>>([
		{
			id: 'all',
			label: tSettingsShowall,
			icon: Settings
		},
		{
			id: 'display',
			label: tSettingsFiledisplay,
			icon: Eye
		},
		{
			id: 'personalization',
			label: tSettingsPersonalization,
			icon: PaintRollerIcon
		},
		{
			id: 'behavior',
			label: tSettingsBehavior,
			icon: MousePointer
		},
		{
			id: 'defaults',
			label: tSettingsDefaultview,
			icon: Layout
		},
		{
			id: 'account',
			label: tSettingsAccount,
			icon: User
		}
	]);

	const sortByOptions = $derived([
		{ value: 'name', label: tSettingsSortbyname },
		{ value: 'size', label: tSettingsSortbysize },
		{ value: 'modTime', label: tSettingsSortbydate },
		{ value: 'type', label: tSettingsSortbytype }
	]);

	const sortDirOptions = $derived([
		{ value: 'asc', label: tSettingsAscending },
		{ value: 'desc', label: tSettingsDescending }
	]);

	const viewModeOptions = $derived([
		{ value: 'list', label: tSettingsListview },
		{ value: 'grid', label: tSettingsGridview }
	]);

	const uiFontOptions = $derived([
		{ value: '', label: tSettingsSystemdefault },
		...FONT_LIST.map(f => ({ value: f.family, label: f.name })),
	]);

	const navButtonClass =
		'group flex w-full cursor-pointer items-center gap-2.5 border-none bg-transparent px-3 py-2 text-left transition-colors duration-100 hover:bg-surface-secondary';
	const activeNavClass = 'bg-selection text-white hover:bg-selection-hover';
	const inactiveNavClass = 'text-text-secondary hover:text-text-primary';
	const toolbarButtonClass =
		'flex h-7 w-7 cursor-pointer items-center justify-center rounded border-none bg-transparent text-text-secondary transition-all duration-100 hover:bg-surface-elevated hover:text-text-primary';
	const panelClass =
		'scroll-mt-4 overflow-hidden rounded-lg border border-border-primary bg-surface-secondary shadow-[0_18px_70px_rgba(0,0,0,0.18)]';
	const panelHeaderClass =
		'flex items-start justify-between gap-4 border-b border-border-secondary bg-surface-primary/55 px-4 py-3';
	const settingRowClass =
		'flex min-h-12 items-center justify-between gap-4 border-b border-border-secondary px-4 py-2 last:border-b-0';

	onDestroy(() => {
		clearApplyProgressResetTimer();
	});

	function clearApplyProgressResetTimer() {
		if (!applyProgressResetTimer) return;

		clearTimeout(applyProgressResetTimer);
		applyProgressResetTimer = null;
	}

	function scheduleApplyProgressReset(delay: number) {
		clearApplyProgressResetTimer();
		applyProgressResetTimer = setTimeout(() => {
			applyProgress = 0;
			applyProgressStatus = '';
			applyProgressVariant = 'default';
			applyProgressResetTimer = null;
		}, delay);
	}

	function waitForPaint(): Promise<void> {
		return new Promise((resolvePaint) => {
			if (typeof requestAnimationFrame === 'undefined') {
				resolvePaint();
				return;
			}

			requestAnimationFrame(() => resolvePaint());
		});
	}

	async function setApplyProgress(value: number, status: string) {
		applyProgress = value;
		applyProgressStatus = status;
		await tick();
		await waitForPaint();
	}

	async function handleSave() {
		if (!accentColorIsValid || !backgroundImageIsValid || isApplyingSettings) return;

		clearApplyProgressResetTimer();
		isApplyingSettings = true;
		applyProgressVariant = 'default';
		let savedLocalBackgroundImage: string | null = null;

		try {
			await setApplyProgress(15, 'Validating settings...');

			const previousBackgroundImage = $settingsStore.backgroundImage;
			let backgroundImage = normalizeBackgroundImage(settings.backgroundImage);
			if (backgroundImage && isInlineWallpaperDataUrl(backgroundImage)) {
				await setApplyProgress(35, 'Saving wallpaper locally...');
				backgroundImage = await saveLocalWallpaperDataUrl(backgroundImage);
				savedLocalBackgroundImage = backgroundImage;
			}

			const nextSettings = {
				...settings,
				accentColor: normalizeAccentColor(settings.accentColor),
				backgroundImage,
				backgroundImageMode: normalizeBackgroundImageMode(settings.backgroundImageMode),
				frostedGlass: backgroundImage ? settings.frostedGlass : false
			};

			await setApplyProgress(
				55,
				backgroundImage ? 'Applying wallpaper and preferences...' : 'Applying preferences...'
			);

			settingsStore.set(nextSettings);
			settings = { ...nextSettings };

			await setApplyProgress(85, 'Refreshing workspace...');
			cleanupLocalWallpaper(previousBackgroundImage, backgroundImage);
			applyProgressVariant = 'success';
			await setApplyProgress(100, 'Settings applied');
			scheduleApplyProgressReset(900);
		} catch (error) {
			cleanupLocalWallpaper(savedLocalBackgroundImage, null);
			applyProgressVariant = 'danger';
			applyProgress = 100;
			applyProgressStatus = getApplyErrorMessage(error);
			scheduleApplyProgressReset(4000);
		} finally {
			isApplyingSettings = false;
		}
	}

	function getApplyErrorMessage(error: unknown): string {
		if (isStorageQuotaError(error)) {
			return 'This wallpaper is too large to save locally. Choose a smaller image or pick one from the server.';
		}

		return error instanceof Error ? error.message : 'Unable to apply settings.';
	}

	function isStorageQuotaError(error: unknown): boolean {
		const errorText = error instanceof Error ? `${error.name} ${error.message}` : String(error);
		return /quota|NS_ERROR_DOM_QUOTA_REACHED|exceeded/i.test(errorText);
	}

	function cleanupLocalWallpaper(
		previousBackgroundImage: string | null,
		nextBackgroundImage: string | null
	) {
		if (
			!previousBackgroundImage ||
			previousBackgroundImage === nextBackgroundImage ||
			!isLocalWallpaperReference(previousBackgroundImage)
		) {
			return;
		}

		deleteLocalWallpaper(previousBackgroundImage).catch(() => {
			// Cleanup failure should not make an already saved preference look failed.
		});
	}

	function handleCancel() {
		clearApplyProgressResetTimer();
		applyProgress = 0;
		applyProgressStatus = '';
		applyProgressVariant = 'default';
		settings = { ...$settingsStore };
	}

	function handleReset() {
		clearApplyProgressResetTimer();
		applyProgress = 0;
		applyProgressStatus = '';
		applyProgressVariant = 'default';
		const previousBackgroundImage = $settingsStore.backgroundImage;
		settingsStore.reset();
		settings = { ...$settingsStore };
		cleanupLocalWallpaper(previousBackgroundImage, null);
	}

	async function handleLogout() {
		await authStore.logout();
		goto(resolve('/login'));
	}

	function goBack() {
		goto(resolve('/browse'));
	}

	function handleSearchInput(query: string) {
		searchQuery = query;
	}

	function handleSearchClear() {
		searchQuery = '';
	}

	function handleAccentColorInput(event: Event) {
		settings.accentColor = (event.currentTarget as HTMLInputElement).value;
	}

	function handleAccentTextInput(event: Event) {
		const value = (event.currentTarget as HTMLInputElement).value.trim();
		settings.accentColor = value === '' ? null : value;
	}

	function handleAccentReset() {
		settings.accentColor = null;
	}

	function matchesSearch(...values: string[]): boolean {
		if (!normalizedSearch) return true;
		return values.some((value) => value.toLowerCase().includes(normalizedSearch));
	}

	function categoryAllows(section: SettingsSectionId): boolean {
		return activeCategory === 'all' || activeCategory === section;
	}

	const showDisplaySection = $derived(
		categoryAllows('display') &&
			matchesSearch(
				'file display',
				'hidden files',
				'file extensions',
				'compact mode',
				'density',
				'lists',
				'grids'
			)
	);
	const showPersonalizationSection = $derived(
		categoryAllows('personalization') &&
			matchesSearch(
				'personalization',
				'accent color',
				'custom color',
				'background image',
				'wallpaper',
				'crop',
				'stretch',
				'fit',
				'wallpaper fit',
				'frosted glass',
				'blur',
				'theme',
				'folder color',
				'selection color'
			)
	);
	const showBehaviorSection = $derived(
		categoryAllows('behavior') &&
			matchesSearch(
				'behavior',
				'confirm before delete',
				'delete',
				'preview on single click',
				'preview'
			)
	);
	const showDefaultsSection = $derived(
		categoryAllows('defaults') &&
			matchesSearch('default view', 'sort by', 'sort direction', 'view mode', 'list', 'grid')
	);
	const showAccountSection = $derived(
		categoryAllows('account') &&
			matchesSearch('account', 'session', 'reset defaults', 'logout', 'local preferences')
	);
	const hasSearchResults = $derived(
		showDisplaySection ||
			showPersonalizationSection ||
			showBehaviorSection ||
			showDefaultsSection ||
			showAccountSection
	);

	// i18n
	let currentLang = $state(getLocale());
</script>

<svelte:head>
	<title>Settings - DockerBox</title>
</svelte:head>

<div class="flex h-screen w-full overflow-hidden bg-surface-primary text-text-primary">
	<aside
		class="flex w-55 min-w-55 flex-col overflow-x-hidden overflow-y-auto border-r border-border-secondary bg-surface-primary"
	>
		<div class="border-b border-border-secondary px-3 py-3">
			<div class="flex items-center gap-2 text-[13px] font-medium text-text-primary">
				<Settings size={16} class="text-accent" />
				<span>{tSettingsTitle}</span>
			</div>
		</div>

		<nav class="flex-1 py-2" aria-label="Settings sections">
			{#each navItems as item (item.id)}
				<button
					type="button"
					class="{navButtonClass} {activeCategory === item.id ? activeNavClass : inactiveNavClass}"
					onclick={() => (activeCategory = item.id)}
					aria-current={activeCategory === item.id ? 'page' : undefined}
				>
					<item.icon size={16} class="mt-0.5 shrink-0 opacity-80" />
					<span class="min-w-0">
						<span class="block text-[13px] leading-5">{item.label}</span>
					</span>
				</button>
			{/each}
		</nav>
	</aside>

	<div class="flex min-w-0 flex-1 flex-col">
		<div
			class="flex items-center gap-2 border-b border-border-secondary bg-surface-primary px-3 py-1.5"
		>
			<button type="button" class={toolbarButtonClass} onclick={goBack} title="Back to files">
				<ChevronLeft size={18} />
			</button>

			<div
				class="flex min-w-0 flex-1 items-center gap-1.5 rounded border border-border-primary bg-surface-secondary px-2 py-1"
			>
				<span class="text-[13px] whitespace-nowrap text-text-secondary">{tSettingsTitle}</span>
				<span class="text-xs text-text-muted">/</span>
				<span class="text-[13px] whitespace-nowrap text-text-primary">{tSettingsPreferences}</span>
			</div>

			<div class="w-64 shrink-0 lg:w-96">
				<SearchBar
					value={searchQuery}
					onInput={handleSearchInput}
					onClear={handleSearchClear}
					placeholder={tSettingsSearchsettings}
					compact
				/>
			</div>

			<div class="flex gap-1">
				<Button
					variant="ghost"
					size="sm"
					onclick={handleCancel}
					title="Discard changes"
					disabled={!hasChanges || isApplyingSettings}
				>
					<X size={16} />
					<span class="hidden sm:inline">{tCommonCancel}</span>
				</Button>
				<ProgressButton
					variant="primary"
					size="sm"
					onclick={handleSave}
					title={applyProgressStatus || tCommonSave}
					disabled={!canSave}
					busy={isApplyingSettings}
					progress={showApplyProgress ? applyProgress : null}
					progressVariant={applyProgressVariant}
					progressLabel={applyProgressStatus}
					className="min-w-20"
				>
					<Save size={16} />
					<span class="hidden sm:inline">{saveButtonText}</span>
				</ProgressButton>
			</div>
		</div>

		<main class="relative flex-1 overflow-auto">
			<div class="relative mx-auto flex max-w-245 flex-col gap-4 px-6 py-6">
				{#if !hasSearchResults}
					<div
						class="rounded-lg border border-border-primary bg-surface-secondary px-4 py-8 text-center"
					>
						<div class="text-sm text-text-primary">No settings found</div>
						<div class="mt-1 text-xs text-text-muted">Try a different search term.</div>
					</div>
				{/if}

				{#if showDisplaySection}
					<section id="display" class={panelClass}>
						<div class={panelHeaderClass}>
							<div>
								<h2 class="m-0 flex items-center gap-2 text-sm font-medium">
									<Eye size={16} class="text-accent" />
									{tSettingsFiledisplay}
								</h2>
							</div>
						</div>

						<div>
							{#if matchesSearch('show hidden files', 'display files and folders that start with a dot')}
								<div class={settingRowClass}>
									<div>
										<div class="text-[13px] text-text-primary">{tSettingsShowhiddenfiles}</div>
									</div>
									<Toggle
										bind:checked={settings.showHiddenFiles}
										label="Show hidden files"
										showLabel={false}
									/>
								</div>
							{/if}

							{#if matchesSearch('show file extensions', 'keep extensions visible in file names')}
								<div class={settingRowClass}>
									<div>
										<div class="text-[13px] text-text-primary">{tSettingsShowfileextensions}</div>
									</div>
									<Toggle
										bind:checked={settings.showFileExtensions}
										label="Show file extensions"
										showLabel={false}
									/>
								</div>
							{/if}

							{#if matchesSearch('compact mode', 'reduce row and tile spacing', 'density')}
								<div class={settingRowClass}>
									<div>
										<div class="text-[13px] text-text-primary">{tSettingsCompactmode}</div>
									</div>
									<Toggle
										bind:checked={settings.compactMode}
										label="Compact mode"
										showLabel={false}
									/>
								</div>
							{/if}
						</div>
					</section>
				{/if}

				{#if showPersonalizationSection}
					<section id="personalization" class={panelClass}>
						<div class={panelHeaderClass}>
							<div>
								<h2 class="m-0 flex items-center gap-2 text-sm font-medium">
									<PaintRollerIcon size={16} class="text-accent" />
									{tSettingsPersonalization}
								</h2>
							</div>
						</div>

						<div>
							<!-- Theme -->
							<div class={settingRowClass}>
								<div class="flex items-center gap-2 text-[13px] text-text-primary">
									{#if settings.theme === 'light'}
										<Sun size={14} class="text-accent" />
									{:else if settings.theme === 'dark'}
										<Moon size={14} class="text-accent" />
									{:else}
										<Settings size={14} class="text-accent" />
									{/if}
									<span>{tSettingsTheme}</span>
								</div>
								<div class="flex items-center gap-1">
									<button
										type="button"
										class="flex items-center gap-1 rounded px-3 py-1 text-xs transition-colors {settings.theme === 'dark' ? 'bg-accent text-white' : 'bg-surface-tertiary text-text-secondary hover:text-text-primary'}"
										onclick={() => { settings.theme = 'dark'; applyTheme('dark'); settingsStore.setSetting('theme', 'dark'); }}
									><Moon size={12} /> {tSettingsDark}</button>
									<button
										type="button"
										class="flex items-center gap-1 rounded px-3 py-1 text-xs transition-colors {settings.theme === 'light' ? 'bg-accent text-white' : 'bg-surface-tertiary text-text-secondary hover:text-text-primary'}"
										onclick={() => { settings.theme = 'light'; applyTheme('light'); settingsStore.setSetting('theme', 'light'); }}
									><Sun size={12} /> {tSettingsLight}</button>
									<button
										type="button"
										class="flex items-center gap-1 rounded px-3 py-1 text-xs transition-colors {settings.theme === 'system' ? 'bg-accent text-white' : 'bg-surface-tertiary text-text-secondary hover:text-text-primary'}"
										onclick={() => { settings.theme = 'system'; applyTheme('system'); settingsStore.setSetting('theme', 'system'); }}
									><Settings size={12} /> {tSettingsSystem}</button>
								</div>
							</div>

							<!-- Language -->
						<div class={settingRowClass}>
							<div class="flex items-center gap-2 text-[13px] text-text-primary">
								<Globe size={14} class="text-accent" />
								<span>{tSettingsLanguage}</span>
							</div>
							<div class="flex items-center gap-2">
								<button type="button" class="rounded px-3 py-1 text-xs transition-colors {currentLang === 'zh-CN' ? 'bg-accent text-white' : 'bg-surface-tertiary text-text-secondary hover:text-text-primary'}" onclick={() => { setLocale('zh-CN'); currentLang = 'zh-CN'; }}>{tSettingsChinese}</button>
								<button type="button" class="rounded px-3 py-1 text-xs transition-colors {currentLang === 'en' ? 'bg-accent text-white' : 'bg-surface-tertiary text-text-secondary hover:text-text-primary'}" onclick={() => { setLocale('en'); currentLang = 'en'; }}>{tSettingsEnglish}</button>
							</div>
						</div>

						{#if matchesSearch('accent color', 'custom color', 'theme', 'folder color', 'selection color')}
							<div class={settingRowClass}>
								<div>
									<div class="flex items-center gap-2 text-[13px] text-text-primary">
										<Palette size={14} class="text-accent" />
										<span>{tSettingsAccentcolor}</span>
										</div>
										{#if !accentColorIsValid}
											<div class="mt-1 text-xs text-danger">Use a #RRGGBB hex color.</div>
										{/if}
									</div>

									<div class="flex flex-wrap items-center justify-end gap-2">
										<input
											type="color"
											value={accentColorValue}
											oninput={handleAccentColorInput}
											aria-label="Choose accent color"
											class="h-8 w-10 cursor-pointer rounded border border-border-primary bg-surface-secondary p-0.5"
										/>
										<input
											type="text"
											value={settings.accentColor ?? ''}
											placeholder={DEFAULT_ACCENT_COLOR}
											oninput={handleAccentTextInput}
											aria-label="Accent color hex value"
											aria-invalid={!accentColorIsValid}
											class="h-8 w-28 rounded border bg-surface-secondary px-2 text-sm text-text-primary placeholder:text-text-muted focus:border-border-focus focus:outline-none {accentColorIsValid
												? 'border-border-primary'
												: 'border-danger'}"
										/>
										<Button variant="secondary" size="sm" onclick={handleAccentReset}
											>{tSettingsDefault}</Button
										>
									</div>
								</div>
							{/if}

							<WallpaperSettings
								rowClass={settingRowClass}
								showHiddenFiles={settings.showHiddenFiles}
								showWallpaperRow={matchesSearch(
									'background image',
									'wallpaper',
									'custom background',
									'personalization'
								)}
								showDisplayModeRow={matchesSearch(
									'wallpaper fit',
									'crop',
									'stretch',
									'fit',
									'center',
									'tile'
								)}
								showFrostedRow={matchesSearch(
									'frosted glass',
									'blur',
									'background blur',
									'glass look'
								)}
								bind:backgroundImage={settings.backgroundImage}
								bind:backgroundImageMode={settings.backgroundImageMode}
								bind:frostedGlass={settings.frostedGlass}
							/>

						{#if matchesSearch('ui font', 'western font', 'interface font', 'display font', 'font', 'font')}
							<div class={settingRowClass}>
								<div>
									<div class="flex items-center gap-2 text-[13px] text-text-primary"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="text-accent"><path d="M13 18L8 6L3 18m8-4H5m16 4v-3m0 0v-3m0 3a3 3 0 1 1-6 0a3 3 0 0 1 6 0"/></svg><span>{tSettingsFont}</span></div>
								</div>
								<div class="w-56">
									<select class="w-full rounded border border-border-secondary bg-surface-secondary px-3 py-1.5 text-xs text-text-primary focus:border-border-focus focus:outline-none" onchange={handleFontChange}>
									{#each uiFontOptions as opt}
										<option value={opt.value} selected={settings.uiFont === opt.value}>{opt.label}</option>
									{/each}
								</select>
								</div>
							</div>
						{/if}


						</div>
					</section>
				{/if}

				{#if showBehaviorSection}
					<section id="behavior" class={panelClass}>
						<div class={panelHeaderClass}>
							<div>
								<h2 class="m-0 flex items-center gap-2 text-sm font-medium">
									<MousePointer size={16} class="text-accent" />
									{tSettingsBehavior}
								</h2>
							</div>
						</div>

						<div>
							{#if matchesSearch('confirm before delete', 'confirmation', 'delete')}
								<div class={settingRowClass}>
									<div>
										<div class="text-[13px] text-text-primary">{tSettingsConfirmbeforedelete}</div>
									</div>
									<Toggle
										bind:checked={settings.confirmDelete}
										label="Confirm before delete"
										showLabel={false}
									/>
								</div>
							{/if}

							{#if matchesSearch('preview on single click', 'preview', 'single click')}
								<div class={settingRowClass}>
									<div>
										<div class="text-[13px] text-text-primary">{tSettingsPreviewonsingleclick}</div>
									</div>
									<Toggle
										bind:checked={settings.previewOnSingleClick}
										label="Preview on single click"
										showLabel={false}
									/>
								</div>
							{/if}
						</div>
					</section>
				{/if}

				{#if showDefaultsSection}
					<section id="defaults" class={panelClass}>
						<div class={panelHeaderClass}>
							<div>
								<h2 class="m-0 flex items-center gap-2 text-sm font-medium">
									<Layout size={16} class="text-accent" />
									{tSettingsDefaultview}
								</h2>
							</div>
						</div>

						<div>
							{#if matchesSearch('sort by', 'default sort field', 'name', 'size', 'date modified', 'type')}
								<div class={settingRowClass}>
									<div>
										<div class="text-[13px] text-text-primary">{tSettingsSortby}</div>
									</div>
									<div class="w-44">
										<Select options={sortByOptions} bind:value={settings.defaultSortBy} />
									</div>
								</div>
							{/if}

							{#if matchesSearch('sort direction', 'ascending', 'descending')}
								<div class={settingRowClass}>
									<div>
										<div class="text-[13px] text-text-primary">{tSettingsSortdirection}</div>
									</div>
									<div class="w-44">
										<Select options={sortDirOptions} bind:value={settings.defaultSortDir} />
									</div>
								</div>
							{/if}

							{#if matchesSearch('view mode', 'list', 'grid')}
								<div class={settingRowClass}>
									<div>
										<div class="text-[13px] text-text-primary">{tSettingsViewmode}</div>
									</div>
									<div class="w-44">
										<Select options={viewModeOptions} bind:value={settings.defaultViewMode} />
									</div>
								</div>
							{/if}
						</div>
					</section>
				{/if}

				{#if showAccountSection}
					<section id="account" class={panelClass}>
						<div class={panelHeaderClass}>
							<div>
								<h2 class="m-0 flex items-center gap-2 text-sm font-medium">
									<User size={16} class="text-accent" />
									{tSettingsAccount}
								</h2>
							</div>
						</div>

						<div class="grid gap-4 p-4 md:grid-cols-[1fr_auto] md:items-center">
							<div>
								<div class="text-[13px] text-text-primary">{tSettingsSignedinsession}</div>
							</div>
							<div class="flex flex-wrap gap-2">
								<Button variant="secondary" size="sm" onclick={handleReset}>
									<RotateCcw size={14} />
									{tSettingsResetdefaults}
								</Button>
								<Button variant="danger" size="sm" onclick={handleLogout}>
									<LogOut size={14} />
									{tSettingsLogout}
								</Button>
							</div>
						</div>
					</section>
				{/if}

				<!-- About -->
				<div class="flex items-center justify-center gap-2 py-6 text-xs text-text-muted">
					<span>DockerBox v0.1.6</span>
					<span>·</span>
					<a
						href="https://github.com/zhaxg/dockerbox"
						target="_blank"
						rel="noopener noreferrer"
						class="text-accent hover:underline"
					>GitHub</a>
				</div>
			</div>
		</main>
	</div>
</div>
