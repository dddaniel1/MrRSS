<script setup lang="ts">
import { useAppStore } from '@/stores/app';
import { useI18n } from 'vue-i18n';
import { ref, computed, onMounted, onBeforeUnmount, watch, nextTick, type Ref } from 'vue';
import {
  PhList,
  PhListDashes,
  PhSquaresFour,
  PhSpinner,
  PhTextIndent,
} from '@phosphor-icons/vue';
import ArticleFilterModal from '../modals/filter/ArticleFilterModal.vue';
import ArticleItem from './ArticleItem.vue';
import ArticleHeaderActions from './ArticleHeaderActions.vue';
import { useArticleTranslation } from '@/composables/article/useArticleTranslation';
import { useArticleFilter } from '@/composables/article/useArticleFilter';
import { useArticleActions } from '@/composables/article/useArticleActions';
import { useShowPreviewImages } from '@/composables/ui/useShowPreviewImages';
import { useSettings } from '@/composables/core/useSettings';
import { buildAutoSavePayload, parseSettingsData } from '@/composables/core/useSettings.generated';
import { openInBrowser } from '@/utils/browser';
import type { Article } from '@/types/models';

const store = useAppStore();
const { t } = useI18n();
const { settings } = useSettings();

const listRef: Ref<HTMLDivElement | null> = ref(null);
const defaultViewMode = ref<'original' | 'rendered' | 'external'>('original');
const showFilterModal = ref(false);
// Track articles that should be temporarily kept in list even if read
const temporarilyKeepArticles = ref<Set<number>>(new Set());
// Flag to control when scroll position should be restored
const shouldRestoreScroll = ref(false);

interface Props {
  isSidebarOpen?: boolean;
}

defineProps<Props>();

const emit = defineEmits<{
  toggleSidebar: [];
}>();

// Use composables
const {
  translationSettings,
  loadTranslationSettings,
  setupIntersectionObserver,
  observeArticle,
  handleTranslationSettingsChange,
  cleanup: cleanupTranslation,
} = useArticleTranslation();

const {
  activeFilters,
  filteredArticlesFromServer,
  isFilterLoading,
  resetFilterState,
  fetchFilteredArticles,
  loadMoreFilteredArticles,
} = useArticleFilter();

// Computed filtered articles - optimized to avoid excessive recomputation
const filteredArticles = computed(() => {
  let articles = activeFilters.value.length > 0 ? filteredArticlesFromServer.value : store.articles;

  // Only apply filter if showOnlyUnread is enabled
  // Using a simpler filter that avoids Set.has() calls when possible
  if (store.showOnlyUnread && temporarilyKeepArticles.value.size > 0) {
    articles = articles.filter(
      (article) => !article.is_read || temporarilyKeepArticles.value.has(article.id)
    );
  } else if (store.showOnlyUnread) {
    // Fast path when no temporarily kept articles
    articles = articles.filter((article) => !article.is_read);
  }

  return articles;
});

const articleLayoutMode = computed<'normal' | 'compact' | 'grid'>(() => {
  const mode = settings.value.article_layout_mode;
  if (mode === 'compact' || mode === 'grid' || mode === 'normal') {
    return mode;
  }

  return settings.value.compact_mode ? 'compact' : 'normal';
});

const currentLayoutIcon = computed(() => {
  if (articleLayoutMode.value === 'compact') {
    return PhTextIndent;
  }
  if (articleLayoutMode.value === 'grid') {
    return PhSquaresFour;
  }
  return PhListDashes;
});

const currentLayoutLabel = computed(() => {
  if (articleLayoutMode.value === 'compact') {
    return t('setting.reading.articleListLayoutCompact');
  }
  if (articleLayoutMode.value === 'grid') {
    return t('setting.reading.articleListLayoutGrid');
  }
  return t('setting.reading.articleListLayoutNormal');
});

const { showArticleContextMenu } = useArticleActions(t, defaultViewMode, async () => {
  await store.fetchUnreadCounts();
  await store.fetchFilterCounts();
});

// Virtual rendering: only render visible articles + buffer
const visibleArticles = computed(() => {
  // For now, render all articles but could be optimized for virtual scrolling
  // Keeping it simple to avoid complexity
  return filteredArticles.value;
});

// Helper to truncate text to max length
function truncateText(text: string, maxLength: number): string {
  if (text.length <= maxLength) return text;
  return text.substring(0, maxLength - 1) + '…';
}

// Dynamic title based on current filter and temporary selection
const articleListTitle = computed(() => {
  // If there's a temporary selection from feed drawer, show feed/category name with filter
  if (store.tempSelection.feedId) {
    const feed = store.feeds?.find((f) => f.id === store.tempSelection.feedId);
    const feedName = feed?.title || '';
    const filterText = getFilterText();

    // Truncate feed name if it's too long (leave room for " - filterText")
    const maxFeedNameLength = filterText ? 40 : 50;
    const truncatedFeedName = truncateText(feedName, maxFeedNameLength);

    return filterText ? `${truncatedFeedName} - ${filterText}` : truncatedFeedName;
  }

  if (store.tempSelection.category) {
    const categoryName = store.tempSelection.category;
    const filterText = getFilterText();

    // Truncate category name if it's too long
    const maxCategoryLength = filterText ? 40 : 50;
    const truncatedCategory = truncateText(categoryName, maxCategoryLength);

    return filterText ? `${truncatedCategory} - ${filterText}` : truncatedCategory;
  }

  // No temporary selection, show filter only
  return getFilterText() || t('sidebar.feedList.articles');
});

// Helper to get filter text
function getFilterText(): string {
  switch (store.currentFilter) {
    case 'all':
      return t('sidebar.activity.allArticles');
    case 'unread':
      return t('sidebar.activity.unreadArticles');
    case 'favorites':
      return t('sidebar.activity.favorites');
    case 'readLater':
      return t('sidebar.activity.readLater');
    case 'imageGallery':
      return t('sidebar.activity.imageGallery');
    case 'videoGallery':
      return t('sidebar.activity.videoGallery');
    default:
      return '';
  }
}

// Initialize show preview images setting
const { initialize: initializeShowPreviewImages } = useShowPreviewImages();

// Load settings and setup
onMounted(async () => {
  await loadTranslationSettings();
  await initializeShowPreviewImages();

  try {
    const res = await fetch('/api/settings');
    const data = await res.json();
    defaultViewMode.value = data.default_view_mode || 'original';

    // Set up intersection observer for auto-translation
    if (translationSettings.value.enabled && listRef.value) {
      setupIntersectionObserver(listRef.value, store.articles);
    }
  } catch (e) {
    console.error('Error loading settings:', e);
  }

  // Listen for translation settings changes
  window.addEventListener(
    'translation-settings-changed',
    onTranslationSettingsChanged as EventListener
  );
  // Listen for default view mode changes
  window.addEventListener('default-view-mode-changed', onDefaultViewModeChanged as EventListener);
  // Listen for show preview images changes
  window.addEventListener(
    'show-preview-images-changed',
    onShowPreviewImagesChanged as EventListener
  );
  // Listen for article layout mode changes
  window.addEventListener('article-layout-mode-changed', onArticleLayoutModeChanged as EventListener);
  // Keep compatibility with legacy compact mode event
  window.addEventListener('compact-mode-changed', onArticleLayoutModeChanged as EventListener);
  // Listen for settings loaded event (from App.vue on startup)
  window.addEventListener('settings-loaded', onSettingsLoaded as EventListener);
  // Listen for refresh articles events
  window.addEventListener('refresh-articles', onRefreshArticles);
  // Listen for toggle filter events (from keyboard shortcut)
  window.addEventListener('toggle-filter', onToggleFilter);
});

// Watch for articles array length changes (list content changes)
watch(
  () => store.articles.length,
  async () => {
    // Only restore scroll position when explicitly needed (e.g., during refresh)
    if (shouldRestoreScroll.value && listRef.value) {
      const currentScroll = listRef.value.scrollTop;
      await nextTick();
      listRef.value.scrollTop = currentScroll;
      shouldRestoreScroll.value = false;
    }
  }
);

// Watch for articles array changes to re-observe new articles for translation
// Use shallow watch to avoid triggering on property changes (like is_read)
watch(
  () => store.articles,
  async () => {
    // Re-setup observer to observe newly added articles
    if (translationSettings.value.enabled && listRef.value) {
      await nextTick();
      setupIntersectionObserver(listRef.value, store.articles);
    }
  }
);

// Watch for filtered articles length changes to re-observe new articles
// Changed from deep watch to length watch for better performance
watch(
  () => filteredArticlesFromServer.value.length,
  async () => {
    // Re-setup observer to observe newly added filtered articles
    if (translationSettings.value.enabled && listRef.value) {
      await nextTick();
      setupIntersectionObserver(listRef.value, filteredArticlesFromServer.value);
    }
  }
);

onBeforeUnmount(() => {
  cleanupTranslation();
  // Clear scroll throttle timer
  if (scrollThrottleTimer) {
    clearTimeout(scrollThrottleTimer);
    scrollThrottleTimer = null;
  }
  window.removeEventListener(
    'translation-settings-changed',
    onTranslationSettingsChanged as EventListener
  );
  window.removeEventListener(
    'default-view-mode-changed',
    onDefaultViewModeChanged as EventListener
  );
  window.removeEventListener(
    'show-preview-images-changed',
    onShowPreviewImagesChanged as EventListener
  );
  window.removeEventListener(
    'article-layout-mode-changed',
    onArticleLayoutModeChanged as EventListener
  );
  window.removeEventListener('compact-mode-changed', onArticleLayoutModeChanged as EventListener);
  window.removeEventListener('settings-loaded', onSettingsLoaded as EventListener);
  window.removeEventListener('refresh-articles', onRefreshArticles);
  window.removeEventListener('toggle-filter', onToggleFilter);
});

interface CustomEventDetail {
  mode?: string;
  enabled?: boolean;
  targetLang?: string;
}

// Event handlers
function onDefaultViewModeChanged(e: Event): void {
  const customEvent = e as CustomEvent<CustomEventDetail>;
  if (customEvent.detail.mode) {
    defaultViewMode.value = customEvent.detail.mode as 'original' | 'rendered';
  }
}

function onTranslationSettingsChanged(e: Event): void {
  const customEvent = e as CustomEvent<CustomEventDetail>;
  const { enabled, targetLang } = customEvent.detail;
  if (enabled !== undefined && targetLang) {
    handleTranslationSettingsChange(enabled, targetLang);

    // Re-setup observer if needed
    if (enabled && listRef.value) {
      setupIntersectionObserver(listRef.value, store.articles);
    }
  }
}

function onShowPreviewImagesChanged(e: Event): void {
  const customEvent = e as CustomEvent<{ value: boolean }>;
  const { updateValue } = useShowPreviewImages();
  updateValue(customEvent.detail.value);
}

function onArticleLayoutModeChanged(): void {
  // Force a re-fetch of settings to update the reactive settings object
  fetch('/api/settings')
    .then((res) => res.json())
    .then((data) => {
      settings.value = parseSettingsData(data);
    })
    .catch((err) => console.error('Error refreshing settings after article layout mode change:', err));
}

async function setArticleLayoutMode(mode: 'normal' | 'compact' | 'grid'): Promise<void> {
  if (articleLayoutMode.value === mode) {
    return;
  }

  try {
    const res = await fetch('/api/settings');
    if (!res.ok) {
      throw new Error('Failed to fetch current settings before update');
    }

    const data = await res.json();
    const parsed = parseSettingsData(data);
    parsed.article_layout_mode = mode;
    parsed.compact_mode = mode === 'compact';

    await fetch('/api/settings', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(buildAutoSavePayload({ value: parsed })),
    });

    settings.value.article_layout_mode = mode;
    settings.value.compact_mode = mode === 'compact';

    window.dispatchEvent(
      new CustomEvent('article-layout-mode-changed', {
        detail: { mode },
      })
    );
    window.dispatchEvent(
      new CustomEvent('compact-mode-changed', {
        detail: { enabled: mode === 'compact' },
      })
    );
  } catch (e) {
    console.error('Failed to update article layout mode:', e);
  }
}

function cycleArticleLayoutMode(): void {
  const nextMode =
    articleLayoutMode.value === 'normal'
      ? 'compact'
      : articleLayoutMode.value === 'compact'
        ? 'grid'
        : 'normal';
  setArticleLayoutMode(nextMode);
}

function onSettingsLoaded(): void {
  // Load initial settings when App.vue has loaded them
  fetch('/api/settings')
    .then((res) => res.json())
    .then((data) => {
      settings.value = parseSettingsData(data);
      console.log('ArticleList settings loaded on startup:', settings.value.article_layout_mode);
    })
    .catch((err) => console.error('Error loading initial settings in ArticleList:', err));
}

function onRefreshArticles(): void {
  store.fetchArticles();
}

function onToggleFilter(): void {
  showFilterModal.value = !showFilterModal.value;
}

// Article selection and interaction
function selectArticle(article: Article): void {
  // Check if we should open in browser based on feed or global settings
  const feed = store.feeds.find((f) => f.id === article.feed_id);
  let openInBrowserMode = false;

  if (feed?.article_view_mode === 'external') {
    openInBrowserMode = true;
  } else if (feed?.article_view_mode === 'global' || !feed?.article_view_mode) {
    // Check global setting
    if (defaultViewMode.value === 'external') {
      openInBrowserMode = true;
    }
  }

  // If external mode is selected, open in browser and mark as read
  if (openInBrowserMode) {
    // Mark as read if not already read
    if (!article.is_read) {
      article.is_read = true;
      fetch(`/api/articles/read?id=${article.id}&read=true`, { method: 'POST' })
        .then(async () => {
          await store.fetchUnreadCounts();
          await store.fetchFilterCounts();
        })
        .catch((e) => {
          console.error('Error marking as read:', e);
        });
    }
    // Open article URL in browser
    openInBrowser(article.url);
    return;
  }

  // Normal article selection - show in app
  // If switching from one article to another, remove the previous one from temp list
  if (store.currentArticleId) {
    temporarilyKeepArticles.value.delete(store.currentArticleId);
  }

  store.currentArticleId = article.id;
  if (!article.is_read) {
    article.is_read = true;
    // Add to temporarily keep list so it doesn't disappear immediately
    temporarilyKeepArticles.value.add(article.id);
    fetch(`/api/articles/read?id=${article.id}&read=true`, { method: 'POST' })
      .then(async () => {
        await store.fetchUnreadCounts();
        await store.fetchFilterCounts();
      })
      .catch((e) => {
        console.error('Error marking as read:', e);
      });
  }
}

// Scrolling handler with throttling to improve performance
let scrollThrottleTimer: ReturnType<typeof setTimeout> | null = null;
const SCROLL_THROTTLE_DELAY = 200; // 200ms throttle
const SCROLL_THRESHOLD = 400; // Increased from 200 to 400 for better UX

function handleScroll(e: Event): void {
  // Throttle scroll events to improve performance
  if (scrollThrottleTimer) return;

  scrollThrottleTimer = setTimeout(() => {
    scrollThrottleTimer = null;

    const target = e.target as HTMLElement;
    const { scrollTop, clientHeight, scrollHeight } = target;

    // Load more when user is within threshold distance from bottom
    if (scrollTop + clientHeight >= scrollHeight - SCROLL_THRESHOLD) {
      if (activeFilters.value.length > 0) {
        loadMoreFilteredArticles();
      } else {
        store.loadMore();
      }
    }
  }, SCROLL_THROTTLE_DELAY);
}

// Filter handlers
async function handleApplyFilters(filters: typeof activeFilters.value): Promise<void> {
  activeFilters.value = filters;
  if (filters.length === 0) {
    resetFilterState();
    store.page = 1;
    shouldRestoreScroll.value = false; // Don't restore scroll when clearing filters
    await store.fetchArticles(false);
  } else {
    shouldRestoreScroll.value = false; // Don't restore scroll when applying filters
    await fetchFilteredArticles(filters, false);
  }
}

// Actions
async function markAllAsRead(): Promise<void> {
  // If filters are active, mark only filtered articles as read
  if (activeFilters.value.length > 0) {
    try {
      // Get IDs of filtered articles
      const articleIds = filteredArticlesFromServer.value.map((a) => a.id);
      if (articleIds.length === 0) {
        window.showToast(t('article.action.noArticlesToMark'), 'info');
        return;
      }

      // Mark all filtered articles as read
      await Promise.all(
        articleIds.map((id) => fetch(`/api/articles/read?id=${id}&read=true`, { method: 'POST' }))
      );

      // Refresh articles and counts
      await store.fetchArticles();
      await store.fetchUnreadCounts();
      await store.fetchFilterCounts();
      window.showToast(t('article.action.markedAllAsRead'), 'success');
    } catch (e) {
      console.error('Error marking filtered articles as read:', e);
    }
  } else {
    // Use store's markAllAsRead which handles feed and category
    const params: { feed_id?: number; category?: string } = {};

    if (store.currentFeedId) {
      params.feed_id = store.currentFeedId;
    } else if (store.currentCategory) {
      params.category = store.currentCategory;
    }

    await store.markAllAsRead(params.feed_id, params.category);
    window.showToast(t('article.action.markedAllAsRead'), 'success');
  }
}

async function clearReadLater(): Promise<void> {
  try {
    const res = await fetch('/api/articles/clear-read-later', { method: 'POST' });
    if (res.ok) {
      await store.fetchArticles();
      await store.fetchFilterCounts();
      window.showToast(t('common.toast.clearedReadLater'), 'success');
    }
  } catch (e) {
    console.error('Error clearing read later:', e);
  }
}

// Handle hover mark as read event from ArticleItem
function handleHoverMarkAsRead(articleId: number): void {
  // Find and update the article in the store
  const article = store.articles.find((a) => a.id === articleId);
  if (article) {
    article.is_read = true;
  }
  // Also update in filtered articles if applicable
  const filteredArticle = filteredArticlesFromServer.value.find((a) => a.id === articleId);
  if (filteredArticle) {
    filteredArticle.is_read = true;
  }
}
</script>

<template>
  <section
    class="article-list flex flex-col w-full border-r border-border bg-bg-primary shrink-0 h-full"
  >
    <div class="p-2 sm:p-4 border-b border-border bg-bg-primary">
      <div class="flex items-center justify-between">
        <h3
          class="m-0 text-base sm:text-lg font-semibold truncate flex-1"
          :title="articleListTitle"
        >
          {{ articleListTitle }}
        </h3>
        <div class="flex items-center gap-1 sm:gap-2">
          <ArticleHeaderActions
            :current-filter="store.currentFilter"
            :show-only-unread="store.showOnlyUnread"
            :active-filters-count="activeFilters.length"
            :layout-icon="currentLayoutIcon"
            :layout-title="currentLayoutLabel"
            @cycle-layout="cycleArticleLayoutMode"
            @clear-read-later="clearReadLater"
            @mark-all-read="markAllAsRead"
            @toggle-show-only-unread="store.toggleShowOnlyUnread()"
            @open-filter="showFilterModal = true"
          />
          <button class="md:hidden text-xl sm:text-2xl p-1" @click="emit('toggleSidebar')">
            <PhList :size="18" class="sm:w-5 sm:h-5" />
          </button>
        </div>
      </div>
    </div>

    <div ref="listRef" class="flex-1 overflow-y-scroll article-list-scroll" @scroll="handleScroll">
      <div
        v-if="filteredArticles.length === 0 && !store.isLoading && !isFilterLoading"
        class="p-4 sm:p-5 text-center text-text-secondary text-sm sm:text-base"
      >
        {{ t('article.content.noArticles') }}
      </div>

      <!-- Article list with content-visibility for performance -->
      <div
        class="article-list-container"
        :class="{ 'article-list-grid': articleLayoutMode === 'grid' }"
      >
        <ArticleItem
          v-for="article in visibleArticles"
          :key="article.id"
          :article="article"
          :is-active="store.currentArticleId === article.id"
          :layout-mode="articleLayoutMode"
          @click="selectArticle(article)"
          @contextmenu="(e) => showArticleContextMenu(e, article)"
          @observe-element="observeArticle"
          @hover-mark-as-read="handleHoverMarkAsRead"
        />
      </div>

      <div
        v-if="store.isLoading || isFilterLoading"
        class="p-3 sm:p-4 text-center text-text-secondary"
      >
        <PhSpinner :size="20" class="animate-spin sm:w-6 sm:h-6" />
      </div>
    </div>
  </section>

  <!-- Filter Modal - Teleported to body to avoid positioning constraints -->
  <Teleport to="body">
    <ArticleFilterModal
      :show="showFilterModal"
      :current-filters="activeFilters"
      @close="showFilterModal = false"
      @apply="handleApplyFilters"
    />
  </Teleport>
</template>

<style scoped>
@reference "../../style.css";

@media (min-width: 768px) {
  .article-list {
    width: var(--article-list-width, 400px);
  }
}

/* Responsive width for article list on medium screens */
@media (max-width: 1400px) and (min-width: 768px) {
  .article-list {
    width: min(var(--article-list-width, 400px), 320px) !important;
  }
}

.animate-spin {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

/* Performance optimization: content-visibility for article list */
.article-list-container {
  content-visibility: auto;
  contain-intrinsic-size: auto 200px;
}

.article-list-grid {
  --article-card-gap: clamp(10px, 1.4vw, 16px);
  @apply p-2 sm:p-3;
  column-width: clamp(150px, 18vw, 220px);
  column-gap: var(--article-card-gap);
}

.article-list-grid :deep(.article-card.grid) {
  break-inside: avoid;
  display: inline-flex;
  width: 100%;
  margin-bottom: var(--article-card-gap);
}

/* Optimize scrolling performance */
.article-list-scroll {
  /* Enable GPU acceleration for smooth scrolling */
  transform: translateZ(0);
  -webkit-transform: translateZ(0);
  /* Optimize scroll performance */
  overflow-anchor: none;
  /* Smooth scrolling behavior */
  scroll-behavior: auto;
}

.article-list {
  /* Enable GPU acceleration for smooth scrolling */
  transform: translateZ(0);
  -webkit-transform: translateZ(0);
}

/* Optimize article card rendering */
.article-card {
  /* Only use will-change when actually animating */
  will-change: auto;
  /* Isolate compositing layers for better performance */
  contain: layout style paint;
  /* Smooth hover transitions */
  transition: background-color 0.15s ease;
}

.article-card:hover {
  /* Enable GPU acceleration during hover */
  transform: translateZ(0);
  -webkit-transform: translateZ(0);
}
</style>
