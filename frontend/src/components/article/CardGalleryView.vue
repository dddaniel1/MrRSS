<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount } from 'vue';
import { useAppStore } from '@/stores/app';
import { useI18n } from 'vue-i18n';
import {
  PhList,
  PhStar,
  PhClockCountdown,
  PhX,
  PhNewspaper,
  PhListDashes,
  PhTextIndent,
  PhSquaresFour,
} from '@phosphor-icons/vue';
import type { Article } from '@/types/models';
import ArticleDetail from './ArticleDetail.vue';
import { buildAutoSavePayload, parseSettingsData } from '@/composables/core/useSettings.generated';
import ArticleFilterModal from '../modals/filter/ArticleFilterModal.vue';
import { useArticleFilter } from '@/composables/article/useArticleFilter';
import ArticleHeaderActions from './ArticleHeaderActions.vue';

interface Props {
  isSidebarOpen?: boolean;
}

defineProps<Props>();

const emit = defineEmits<{
  toggleSidebar: [];
}>();

const store = useAppStore();
const { t } = useI18n();
const showFilterModal = ref(false);

const {
  activeFilters,
  filteredArticlesFromServer,
  isFilterLoading,
  filterHasMore,
  resetFilterState,
  fetchFilteredArticles,
  loadMoreFilteredArticles,
} = useArticleFilter();

const BASE_MIN_CARD_WIDTH = 220;
const CARD_GRID_GAP_PX = 16;
const CARD_SCROLL_THRESHOLD_PX = 500;
const MAX_CARD_COLUMNS = 7;

const galleryContainerRef = ref<HTMLElement | null>(null);
const cardGridRef = ref<HTMLElement | null>(null);
const cardColumns = ref(1);
let cardResizeObserver: { disconnect: () => void; observe: Function } | null = null;
const scrollLoadInFlight = ref(false);

type ArticleWithBodyFields = Article & {
  content?: string;
  translated_content?: string;
  description?: string;
  translated_description?: string;
};

const showArticleDetailOverlay = ref(false);
const articleLayoutMode = ref<'normal' | 'compact' | 'grid'>('grid');

const currentLayoutIcon = computed(() => {
  if (articleLayoutMode.value === 'compact') return PhTextIndent;
  if (articleLayoutMode.value === 'grid') return PhSquaresFour;
  return PhListDashes;
});

const currentLayoutLabel = computed(() => {
  if (articleLayoutMode.value === 'compact') return t('setting.reading.articleListLayoutCompact');
  if (articleLayoutMode.value === 'grid') return t('setting.reading.articleListLayoutGrid');
  return t('setting.reading.articleListLayoutNormal');
});

function stripHtml(input: string): string {
  return input
    .replace(/<[^>]*>/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
}

function truncateText(input: string, maxLength: number): string {
  if (input.length <= maxLength) return input;
  return `${input.slice(0, maxLength).trimEnd()}...`;
}

function getBodyText(article: Article): string {
  const item = article as ArticleWithBodyFields;
  const raw =
    item.translated_content ||
    item.content ||
    item.translated_description ||
    item.description ||
    item.summary ||
    '';
  return raw ? stripHtml(raw) : '';
}

function formatDate(dateString: string): string {
  const date = new Date(dateString);
  const now = new Date();
  const diff = now.getTime() - date.getTime();
  const days = Math.floor(diff / (1000 * 60 * 60 * 24));

  if (days === 0) {
    const hours = Math.floor(diff / (1000 * 60 * 60));
    if (hours === 0) {
      const minutes = Math.floor(diff / (1000 * 60));
      return minutes <= 0
        ? t('common.time.justNow')
        : t('common.time.minutesAgo', { count: minutes });
    }
    return t('common.time.hoursAgo', { count: hours });
  }

  if (days < 7) {
    return t('common.time.daysAgo', { count: days });
  }

  return date.toLocaleDateString();
}

function hashString(input: string): number {
  let hash = 0;
  for (let i = 0; i < input.length; i += 1) {
    hash = (hash << 5) - hash + input.charCodeAt(i);
    hash |= 0;
  }
  return Math.abs(hash);
}

function getPlaceholderStyle(article: Article): Record<string, string> {
  const seed = `${article.feed_title || ''}-${article.title || ''}-${article.id}`;
  const hue = hashString(seed) % 360;
  const hue2 = (hue + 34) % 360;

  return {
    background: `linear-gradient(140deg, hsl(${hue} 72% 62%), hsl(${hue2} 68% 48%))`,
  };
}

const filteredArticles = computed(() => {
  let articles =
    activeFilters.value.length > 0 ? [...filteredArticlesFromServer.value] : [...store.articles];

  if (store.showOnlyUnread) {
    articles = articles.filter((a) => !a.is_read);
  }

  switch (store.currentFilter) {
    case 'unread':
      articles = articles.filter((a) => !a.is_read);
      break;
    case 'favorites':
      articles = articles.filter((a) => a.is_favorite);
      break;
    case 'readLater':
      articles = articles.filter((a) => a.is_read_later);
      break;
    default:
      break;
  }

  return articles.sort(
    (a, b) => new Date(b.published_at).getTime() - new Date(a.published_at).getTime()
  );
});

const cardGridStyle = computed(() => ({
  gridTemplateColumns: `repeat(${cardColumns.value}, minmax(0, 1fr))`,
}));

const isLoadingMore = computed(() => {
  if (activeFilters.value.length > 0) {
    return isFilterLoading.value;
  }
  return store.isLoading;
});

const hasMoreArticles = computed(() => {
  if (activeFilters.value.length > 0) {
    return filterHasMore.value;
  }
  return store.hasMore;
});

function updateCardColumns(): void {
  const target = cardGridRef.value ?? galleryContainerRef.value;
  const width = target?.clientWidth;
  if (!width || width <= 0) {
    return;
  }

  const styles = target ? window.getComputedStyle(target) : null;
  const paddingLeft = Number.parseFloat(styles?.paddingLeft || '0') || 0;
  const paddingRight = Number.parseFloat(styles?.paddingRight || '0') || 0;
  const effectiveGap =
    Number.parseFloat(styles?.columnGap || styles?.gap || '0') || CARD_GRID_GAP_PX;
  const contentWidth = Math.max(0, width - paddingLeft - paddingRight);

  const calculated = Math.floor(
    (contentWidth + effectiveGap) / (BASE_MIN_CARD_WIDTH + effectiveGap)
  );
  cardColumns.value = Math.min(MAX_CARD_COLUMNS, Math.max(1, calculated));
}

function setupCardResizeObserver(): void {
  const ResizeObserverConstructor = window.ResizeObserver;
  if (!ResizeObserverConstructor || !galleryContainerRef.value) {
    return;
  }

  if (!cardResizeObserver) {
    cardResizeObserver = new ResizeObserverConstructor(() => {
      updateCardColumns();
    });
  }

  cardResizeObserver.disconnect();
  cardResizeObserver.observe(galleryContainerRef.value);
}

async function handleGalleryScroll(event: Event): Promise<void> {
  const target = event.target as HTMLElement;
  if (!target) {
    return;
  }

  if (scrollLoadInFlight.value || isLoadingMore.value || !hasMoreArticles.value) {
    return;
  }

  const { scrollTop, clientHeight, scrollHeight } = target;
  if (scrollTop + clientHeight < scrollHeight - CARD_SCROLL_THRESHOLD_PX) {
    return;
  }

  scrollLoadInFlight.value = true;
  try {
    if (activeFilters.value.length > 0) {
      await loadMoreFilteredArticles();
    } else {
      await store.loadMore();
    }
  } finally {
    scrollLoadInFlight.value = false;
  }
}

async function markAsRead(article: Article) {
  if (article.is_read) return;
  try {
    const res = await fetch(`/api/articles/read?id=${article.id}&read=true`, { method: 'POST' });
    if (res.ok) {
      article.is_read = true;
      await store.fetchUnreadCounts();
      await store.fetchFilterCounts();
    }
  } catch (e) {
    console.error('Failed to mark article as read:', e);
  }
}

async function markAllAsRead(): Promise<void> {
  if (activeFilters.value.length > 0) {
    try {
      const articleIds = filteredArticlesFromServer.value.map((a) => a.id);
      if (articleIds.length === 0) {
        window.showToast(t('article.action.noArticlesToMark'), 'info');
        return;
      }

      await Promise.all(
        articleIds.map((id) => fetch(`/api/articles/read?id=${id}&read=true`, { method: 'POST' }))
      );

      await store.fetchArticles();
      await store.fetchUnreadCounts();
      await store.fetchFilterCounts();
      window.showToast(t('article.action.markedAllAsRead'), 'success');
    } catch (e) {
      console.error('Error marking filtered articles as read:', e);
    }
  } else {
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

async function openCardDetail(article: Article) {
  // Force rendered-content mode for card click detail
  store.articleViewModePreferences.set(article.id, 'rendered');

  showArticleDetailOverlay.value = true;
  store.currentArticleId = article.id;

  if (!article.is_read) {
    await markAsRead(article);
  }
}

function closeCardDetail() {
  showArticleDetailOverlay.value = false;
  store.currentArticleId = null;
}

async function setArticleLayoutMode(mode: 'normal' | 'compact' | 'grid'): Promise<void> {
  if (articleLayoutMode.value === mode) {
    return;
  }

  try {
    const res = await fetch('/api/settings');
    if (!res.ok) {
      throw new Error('Failed to fetch current settings before layout update');
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
    articleLayoutMode.value = mode;
  } catch (e) {
    console.error('Failed to update article layout mode in card view:', e);
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

async function handleApplyFilters(filters: typeof activeFilters.value): Promise<void> {
  activeFilters.value = filters;
  if (filters.length === 0) {
    resetFilterState();
    store.page = 1;
    await store.fetchArticles(false);
  } else {
    await fetchFilteredArticles(filters, false);
  }
}

function onToggleFilter(): void {
  showFilterModal.value = !showFilterModal.value;
}

function onCardDetailKeydown(event: KeyboardEvent): void {
  if (!showArticleDetailOverlay.value) {
    return;
  }

  if (event.key === 'Escape') {
    closeCardDetail();
  }
}

async function loadLayoutModeFromSettings(): Promise<void> {
  try {
    const res = await fetch('/api/settings');
    if (!res.ok) {
      return;
    }
    const data = await res.json();
    const parsed = parseSettingsData(data);
    if (
      parsed.article_layout_mode === 'normal' ||
      parsed.article_layout_mode === 'compact' ||
      parsed.article_layout_mode === 'grid'
    ) {
      articleLayoutMode.value = parsed.article_layout_mode;
    } else {
      articleLayoutMode.value = parsed.compact_mode ? 'compact' : 'normal';
    }
  } catch (e) {
    console.error('Failed to load article layout mode in card view:', e);
  }
}

function onArticleLayoutModeChanged(e: Event): void {
  const customEvent = e as CustomEvent<{ mode?: 'normal' | 'compact' | 'grid' }>;
  const mode = customEvent.detail?.mode;
  if (mode === 'normal' || mode === 'compact' || mode === 'grid') {
    articleLayoutMode.value = mode;
  }
}

onMounted(() => {
  loadLayoutModeFromSettings();
  updateCardColumns();
  setupCardResizeObserver();
  window.addEventListener('resize', updateCardColumns);
  window.addEventListener('keydown', onCardDetailKeydown);
  window.addEventListener('toggle-filter', onToggleFilter);
  window.addEventListener(
    'article-layout-mode-changed',
    onArticleLayoutModeChanged as EventListener
  );
});

onBeforeUnmount(() => {
  if (cardResizeObserver) {
    cardResizeObserver.disconnect();
    cardResizeObserver = null;
  }
  window.removeEventListener('resize', updateCardColumns);
  window.removeEventListener('keydown', onCardDetailKeydown);
  window.removeEventListener('toggle-filter', onToggleFilter);
  window.removeEventListener(
    'article-layout-mode-changed',
    onArticleLayoutModeChanged as EventListener
  );
});
</script>

<template>
  <div class="flex flex-col flex-1 h-full bg-bg-primary">
    <div
      class="flex-shrink-0 bg-bg-primary border-b border-border p-2 sm:p-4 flex items-center gap-3"
    >
      <button
        class="p-2 rounded-lg hover:bg-bg-tertiary text-text-primary transition-colors md:hidden"
        :title="t('shortcut.toggle.sidebar')"
        @click="emit('toggleSidebar')"
      >
        <PhList :size="24" />
      </button>
      <h1 class="text-base sm:text-lg font-bold text-text-primary line-height-fixed-32">
        {{ t('setting.reading.articleListLayoutGrid') }}
      </h1>

      <div class="ml-auto flex items-center gap-1 sm:gap-2">
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
      </div>
    </div>

    <div
      ref="galleryContainerRef"
      class="flex-1 overflow-y-scroll scroll-smooth"
      @scroll="handleGalleryScroll"
    >
      <div
        v-if="filteredArticles.length > 0"
        ref="cardGridRef"
        class="p-4 card-gallery-grid"
        :style="cardGridStyle"
      >
        <article
          v-for="article in filteredArticles"
          :key="article.id"
          class="card-gallery-item rounded-xl border border-border bg-bg-secondary shadow-sm overflow-hidden cursor-pointer"
          @click="openCardDetail(article)"
        >
          <img
            v-if="article.image_url"
            :src="article.image_url"
            :alt="article.title"
            class="card-gallery-image w-full object-cover block"
            loading="lazy"
          />
          <div
            v-else
            class="card-gallery-image card-gallery-image-placeholder"
            :style="getPlaceholderStyle(article)"
          >
            <div class="placeholder-orb placeholder-orb-a"></div>
            <div class="placeholder-orb placeholder-orb-b"></div>
            <div class="placeholder-icon-wrap">
              <PhNewspaper :size="20" weight="duotone" />
            </div>
            <p class="placeholder-feed-title line-clamp-1">
              {{ article.feed_title || t('sidebar.feeds.allFeeds') }}
            </p>
          </div>

          <div class="card-gallery-body">
            <h3 class="text-sm sm:text-base font-semibold text-text-primary line-clamp-2 mb-2">
              {{ article.translated_title || article.title }}
            </h3>
            <p class="text-xs sm:text-sm text-text-secondary line-clamp-3 mb-2">
              {{ truncateText(getBodyText(article), 180) }}
            </p>
            <div class="flex items-center justify-between text-xs text-text-secondary">
              <span class="truncate mr-2">{{ article.feed_title }}</span>
              <span class="shrink-0">{{ formatDate(article.published_at) }}</span>
            </div>
            <div class="mt-2 flex items-center gap-2 text-text-secondary">
              <PhClockCountdown
                v-if="article.is_read_later"
                :size="14"
                class="text-blue-500"
                weight="fill"
              />
              <PhStar v-if="article.is_favorite" :size="14" class="text-yellow-500" weight="fill" />
            </div>
          </div>
        </article>
      </div>

      <div v-if="isLoadingMore" class="pb-6 flex justify-center">
        <div
          class="w-8 h-8 border-4 border-accent border-t-transparent rounded-full animate-spin"
        ></div>
      </div>

      <div
        v-else-if="filteredArticles.length === 0 && !isLoadingMore"
        class="h-full flex items-center justify-center text-text-secondary"
      >
        {{ t('article.content.noArticles') }}
      </div>
    </div>

    <div
      v-if="showArticleDetailOverlay && store.currentArticleId"
      class="card-detail-overlay fixed inset-0 z-50 p-2 sm:p-5 flex items-stretch sm:items-center justify-center"
      @click="closeCardDetail"
    >
      <div
        class="card-detail-shell w-full h-full sm:h-[min(92vh,calc(100vh-3rem))] sm:max-w-[1200px]"
        @click.stop
      >
        <button
          type="button"
          class="card-detail-close absolute top-3 left-3 z-[70] p-1.5 sm:p-2 rounded-lg transition-colors flex items-center justify-center text-text-secondary hover:text-text-primary hover:bg-bg-tertiary"
          :title="t('common.close')"
          :aria-label="t('common.close')"
          @click="closeCardDetail"
        >
          <PhX :size="20" class="sm:w-6 sm:h-6" />
        </button>
        <div class="card-detail-content h-full w-full overflow-hidden rounded-[inherit]">
          <ArticleDetail />
        </div>
      </div>
    </div>

    <Teleport to="body">
      <ArticleFilterModal
        :show="showFilterModal"
        :current-filters="activeFilters"
        @close="showFilterModal = false"
        @apply="handleApplyFilters"
      />
    </Teleport>
  </div>
</template>

<style scoped>
.card-gallery-grid {
  --card-gap: clamp(12px, 1.6vw, 20px);
  display: grid;
  grid-template-columns: repeat(1, minmax(0, 1fr));
  gap: var(--card-gap);
}

.card-gallery-item {
  display: flex;
  flex-direction: column;
  aspect-ratio: 1 / 1;
}

.card-gallery-image {
  height: 44%;
}

.card-gallery-image-placeholder {
  position: relative;
  overflow: hidden;
  color: rgba(255, 255, 255, 0.95);
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  padding: 0.6rem;
}

.placeholder-orb {
  position: absolute;
  border-radius: 999px;
  pointer-events: none;
  opacity: 0.35;
  background: rgba(255, 255, 255, 0.36);
  filter: blur(2px);
}

.placeholder-orb-a {
  width: 66px;
  height: 66px;
  top: -16px;
  right: -10px;
}

.placeholder-orb-b {
  width: 52px;
  height: 52px;
  bottom: -14px;
  left: -8px;
}

.placeholder-icon-wrap {
  position: relative;
  width: 30px;
  height: 30px;
  border-radius: 999px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.18);
  border: 1px solid rgba(255, 255, 255, 0.28);
}

.placeholder-feed-title {
  position: relative;
  font-size: 11px;
  line-height: 1.2;
  font-weight: 600;
  letter-spacing: 0.01em;
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.25);
}

.card-gallery-body {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  padding: 0.75rem;
}

.card-gallery-body > p {
  flex: 1;
}

.card-detail-overlay {
  background:
    radial-gradient(circle at top center, rgba(120, 160, 255, 0.16), transparent 46%),
    color-mix(in srgb, black 62%, transparent);
  backdrop-filter: blur(10px) saturate(112%);
}

.card-detail-shell {
  position: relative;
  border-radius: 0;
  border: 1px solid color-mix(in srgb, var(--color-border) 72%, white 28%);
  background: color-mix(in srgb, var(--color-bg-primary) 95%, black 5%);
  overflow: hidden;
  animation: card-detail-pop-in 220ms cubic-bezier(0.2, 0.75, 0.2, 1);
  box-shadow:
    0 32px 88px rgba(0, 0, 0, 0.5),
    0 10px 24px rgba(0, 0, 0, 0.26),
    inset 0 1px 0 rgba(255, 255, 255, 0.07);
}

.card-detail-shell::before {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: inherit;
  pointer-events: none;
  background: linear-gradient(
    to bottom,
    color-mix(in srgb, var(--color-bg-secondary) 70%, transparent),
    transparent 16%
  );
  opacity: 0.38;
}

@keyframes card-detail-pop-in {
  from {
    opacity: 0;
    transform: translateY(10px) scale(0.985);
  }
  to {
    opacity: 1;
    transform: translateY(0) scale(1);
  }
}

@media (prefers-reduced-motion: reduce) {
  .card-detail-shell {
    animation: none;
  }
}

@media (min-width: 640px) {
  .card-detail-shell {
    border-radius: 1rem;
  }
}
</style>
