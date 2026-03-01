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
  resetFilterState,
  fetchFilteredArticles,
} = useArticleFilter();

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
  return input.replace(/<[^>]*>/g, ' ').replace(/\s+/g, ' ').trim();
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
  let articles = activeFilters.value.length > 0 ? [...filteredArticlesFromServer.value] : [...store.articles];

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
  window.addEventListener('toggle-filter', onToggleFilter);
  window.addEventListener('article-layout-mode-changed', onArticleLayoutModeChanged as EventListener);
});

onBeforeUnmount(() => {
  window.removeEventListener('toggle-filter', onToggleFilter);
  window.removeEventListener(
    'article-layout-mode-changed',
    onArticleLayoutModeChanged as EventListener
  );
});
</script>

<template>
  <div class="flex flex-col flex-1 h-full bg-bg-primary">
    <div class="flex-shrink-0 bg-bg-primary border-b border-border p-2 sm:p-4 flex items-center gap-3">
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

    <div class="flex-1 overflow-y-scroll scroll-smooth">
      <div v-if="filteredArticles.length > 0" class="p-4 card-gallery-columns">
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
              <PhClockCountdown v-if="article.is_read_later" :size="14" class="text-blue-500" weight="fill" />
              <PhStar v-if="article.is_favorite" :size="14" class="text-yellow-500" weight="fill" />
            </div>
          </div>
        </article>
      </div>

      <div v-else class="h-full flex items-center justify-center text-text-secondary">
        {{ t('article.content.noArticles') }}
      </div>
    </div>

    <div
      v-if="showArticleDetailOverlay && store.currentArticleId"
      class="card-detail-overlay fixed inset-0 z-50 p-2 sm:p-5"
      @click="closeCardDetail"
    >
      <div class="card-detail-shell w-full h-full sm:h-[calc(100vh-2.5rem)] sm:max-w-6xl sm:mx-auto" @click.stop>
        <button
          class="card-detail-close absolute top-3 left-3 z-[70] w-9 h-9 rounded-full transition-colors hidden sm:flex items-center justify-center"
          :title="t('common.close')"
          @click="closeCardDetail"
        >
          <PhX :size="18" />
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
.card-gallery-columns {
  --card-gap: clamp(12px, 1.6vw, 20px);
  column-width: clamp(150px, 16vw, 220px);
  column-gap: var(--card-gap);
}

.card-gallery-item {
  break-inside: avoid;
  display: inline-flex;
  flex-direction: column;
  width: 100%;
  margin-bottom: var(--card-gap);
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
  background: color-mix(in srgb, black 58%, transparent);
  backdrop-filter: blur(6px);
}

.card-detail-shell {
  position: relative;
  border-radius: 1rem;
  border: 1px solid color-mix(in srgb, var(--color-border) 82%, white 18%);
  background: var(--color-bg-primary);
  box-shadow:
    0 24px 64px rgba(0, 0, 0, 0.42),
    0 4px 14px rgba(0, 0, 0, 0.22);
}

.card-detail-shell::before {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: inherit;
  pointer-events: none;
  background: linear-gradient(
    to bottom,
    color-mix(in srgb, var(--color-bg-secondary) 58%, transparent),
    transparent 18%
  );
  opacity: 0.45;
}

.card-detail-close {
  border: 1px solid color-mix(in srgb, var(--color-border) 78%, white 22%);
  background: color-mix(in srgb, var(--color-bg-tertiary) 84%, black 16%);
  color: var(--color-text-primary);
}

.card-detail-close:hover {
  background: color-mix(in srgb, var(--color-bg-tertiary) 96%, black 4%);
}
</style>
