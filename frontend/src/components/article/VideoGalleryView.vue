<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch, nextTick } from 'vue';
import { useAppStore } from '@/stores/app';
import { useI18n } from 'vue-i18n';
import type { Article } from '@/types/models';
import { PhPlay, PhHeart, PhList, PhX } from '@phosphor-icons/vue';
import { openInBrowser } from '@/utils/browser';
import { getProxiedMediaUrl } from '@/utils/mediaProxy';
import VideoPlayer from './parts/VideoPlayer.vue';

const store = useAppStore();
const { t } = useI18n();

interface Props {
  isSidebarOpen?: boolean;
}

defineProps<Props>();

const emit = defineEmits<{
  toggleSidebar: [];
}>();

const BASE_ITEMS_PER_PAGE = 30;
const MIN_CARD_WIDTH = 220;
const GRID_GAP_PX = 16;
const SCROLL_THRESHOLD_PX = 500;

const articles = ref<Article[]>([]);
const isLoading = ref(false);
const page = ref(1);
const hasMore = ref(true);
const itemsPerPage = ref(BASE_ITEMS_PER_PAGE);
const selectedArticle = ref<Article | null>(null);
const showVideoViewer = ref(false);
const containerRef = ref<HTMLElement | null>(null);

const feedId = computed(() => store.currentFeedId);
const category = computed(() => store.currentCategory);

const sortedArticles = computed(() =>
  [...articles.value].sort((a, b) => {
    return new Date(b.published_at).getTime() - new Date(a.published_at).getTime();
  })
);

function getYouTubeThumbnail(videoUrl?: string): string {
  if (!videoUrl) return '';
  const embedMatch = videoUrl.match(/youtube\.com\/embed\/([^?&/]+)/i);
  if (embedMatch && embedMatch[1]) {
    return `https://img.youtube.com/vi/${embedMatch[1]}/hqdefault.jpg`;
  }
  const shortMatch = videoUrl.match(/youtu\.be\/([^?&/]+)/i);
  if (shortMatch && shortMatch[1]) {
    return `https://img.youtube.com/vi/${shortMatch[1]}/hqdefault.jpg`;
  }
  return '';
}

function getVideoCover(article: Article): string {
  const fallback = getYouTubeThumbnail(article.video_url);
  const raw = article.image_url || fallback;
  if (!raw) return '';
  return getProxiedMediaUrl(raw, article.url);
}

async function fetchVideos(loadMore = false) {
  if (isLoading.value) return;
  isLoading.value = true;

  try {
    let url = `/api/articles/videos?page=${page.value}&limit=${itemsPerPage.value}`;
    if (feedId.value) {
      url += `&feed_id=${feedId.value}`;
    } else if (category.value !== null) {
      url += `&category=${encodeURIComponent(category.value)}`;
    }

    const res = await fetch(url);
    if (res.ok) {
      const data = await res.json();
      if (Array.isArray(data)) {
        if (loadMore) {
          articles.value = [...articles.value, ...data];
        } else {
          articles.value = data;
        }
        hasMore.value = data.length >= itemsPerPage.value;
      }
    }
  } catch (e) {
    console.error('Failed to load videos:', e);
  } finally {
    isLoading.value = false;
  }
}

function updateItemsPerPage() {
  if (!containerRef.value) return;
  const { clientWidth, clientHeight } = containerRef.value;
  if (clientWidth === 0 || clientHeight === 0) return;

  const columns = Math.max(1, Math.floor((clientWidth + GRID_GAP_PX) / (MIN_CARD_WIDTH + GRID_GAP_PX)));
  const cardHeight = (MIN_CARD_WIDTH * 9) / 16 + 56;
  const rows = Math.max(2, Math.floor((clientHeight + GRID_GAP_PX) / (cardHeight + GRID_GAP_PX)));
  const next = Math.max(BASE_ITEMS_PER_PAGE, columns * rows * 2);
  itemsPerPage.value = next;
}

function handleScroll() {
  if (!containerRef.value || isLoading.value || !hasMore.value) return;
  const { scrollTop, clientHeight, scrollHeight } = containerRef.value;
  if (scrollTop + clientHeight >= scrollHeight - SCROLL_THRESHOLD_PX) {
    page.value += 1;
    fetchVideos(true);
  }
}

function handleCardClick(article: Article) {
  selectedArticle.value = article;
  showVideoViewer.value = true;
  if (!article.is_read) {
    markAsRead(article);
  }
}

async function markAsRead(article: Article) {
  try {
    const res = await fetch(`/api/articles/read?id=${article.id}&read=true`, { method: 'POST' });
    if (res.ok) {
      article.is_read = true;
      await store.fetchUnreadCounts();
      await store.fetchFilterCounts();
    }
  } catch (e) {
    console.error('Failed to mark as read:', e);
  }
}

async function toggleFavorite(article: Article, event?: Event) {
  if (event) event.stopPropagation();
  try {
    const res = await fetch(`/api/articles/favorite?id=${article.id}`, { method: 'POST' });
    if (res.ok) {
      article.is_favorite = !article.is_favorite;
      await store.fetchFilterCounts();
    }
  } catch (e) {
    console.error('Failed to toggle favorite:', e);
  }
}

function closeVideoViewer() {
  showVideoViewer.value = false;
  selectedArticle.value = null;
}

function openOriginal(article: Article) {
  openInBrowser(article.url);
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
  } else if (days < 7) {
    return t('common.time.daysAgo', { count: days });
  }
  return date.toLocaleDateString();
}

function handleKeyDown(e: KeyboardEvent) {
  if (!showVideoViewer.value) return;
  if (e.key === 'Escape') {
    closeVideoViewer();
  }
}

watch(feedId, async () => {
  closeVideoViewer();
  page.value = 1;
  articles.value = [];
  hasMore.value = true;
  updateItemsPerPage();
  await fetchVideos();
  await nextTick();
});

watch(category, async () => {
  closeVideoViewer();
  page.value = 1;
  articles.value = [];
  hasMore.value = true;
  updateItemsPerPage();
  await fetchVideos();
  await nextTick();
});

onMounted(() => {
  updateItemsPerPage();
  fetchVideos();
  if (containerRef.value) {
    containerRef.value.addEventListener('scroll', handleScroll);
  }
  window.addEventListener('keydown', handleKeyDown);
  window.addEventListener('resize', updateItemsPerPage);
});

onUnmounted(() => {
  if (containerRef.value) {
    containerRef.value.removeEventListener('scroll', handleScroll);
  }
  window.removeEventListener('keydown', handleKeyDown);
  window.removeEventListener('resize', updateItemsPerPage);
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
      <div class="flex items-center gap-2 flex-1">
        <h1 class="text-base sm:text-lg font-bold text-text-primary line-height-fixed-32">
          {{ t('sidebar.activity.videoGallery') }}
        </h1>
      </div>
    </div>

    <div ref="containerRef" class="flex-1 overflow-y-scroll scroll-smooth">
      <div v-if="articles.length > 0" class="p-4 video-gallery-grid">
        <div
          v-for="article in sortedArticles"
          :key="article.id"
          class="video-gallery-card group"
          @click="handleCardClick(article)"
        >
          <div class="relative overflow-hidden rounded-lg bg-bg-secondary video-gallery-media">
            <img
              v-if="getVideoCover(article)"
              :src="getVideoCover(article)"
              :alt="article.title"
              class="w-full h-full object-cover block"
              loading="lazy"
            />
            <div v-else class="w-full h-full flex items-center justify-center bg-bg-tertiary">
              <PhPlay :size="28" class="text-text-secondary" />
            </div>
            <div class="absolute inset-0 bg-black/20 opacity-0 group-hover:opacity-100 transition-opacity"></div>
            <div class="absolute inset-0 flex items-center justify-center">
              <div
                class="w-12 h-12 rounded-full bg-black/60 text-white flex items-center justify-center shadow-lg"
              >
                <PhPlay :size="24" weight="fill" />
              </div>
            </div>
            <button
              class="absolute top-2 right-2 w-8 h-8 rounded-full bg-black/60 text-white flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity"
              :title="
                article.is_favorite
                  ? t('article.action.removeFromFavorites')
                  : t('article.action.addToFavorites')
              "
              @click.stop="toggleFavorite(article, $event)"
            >
              <PhHeart
                :size="16"
                :weight="article.is_favorite ? 'fill' : 'regular'"
                :class="article.is_favorite ? 'text-red-500' : 'text-white'"
              />
            </button>
          </div>
          <div class="p-2">
            <p class="text-sm font-semibold text-text-primary line-clamp-2 mb-1">
              {{ article.title }}
            </p>
            <div class="flex items-center justify-between text-xs text-text-secondary">
              <span class="truncate flex-1">{{ article.feed_title }}</span>
              <span class="ml-2 shrink-0">{{ formatDate(article.published_at) }}</span>
            </div>
          </div>
        </div>
      </div>

      <div v-else-if="!isLoading" class="flex flex-col items-center justify-center h-full gap-4">
        <PhPlay :size="64" class="text-text-secondary opacity-50" />
        <p class="text-text-secondary">{{ t('article.content.noArticles') }}</p>
      </div>

      <div v-if="isLoading" class="flex justify-center py-8">
        <div class="w-8 h-8 border-4 border-accent border-t-transparent rounded-full animate-spin"></div>
      </div>
    </div>

    <div
      v-if="showVideoViewer && selectedArticle"
      class="fixed inset-0 z-50 bg-black/80 flex flex-col p-4"
      @click="closeVideoViewer"
    >
      <div
        class="video-viewer-shell mx-auto w-full bg-bg-primary rounded-xl border border-border shadow-2xl flex flex-col overflow-hidden"
        @click.stop
      >
        <div class="flex items-center justify-between px-4 py-3 border-b border-border">
          <h2 class="text-base font-semibold text-text-primary line-clamp-1">
            {{ selectedArticle.title }}
          </h2>
          <button
            class="w-8 h-8 rounded-full hover:bg-bg-tertiary text-text-secondary flex items-center justify-center"
            :title="t('common.close')"
            @click="closeVideoViewer"
          >
            <PhX :size="18" />
          </button>
        </div>

        <div class="px-4 pt-4">
          <VideoPlayer
            v-if="selectedArticle.video_url"
            :video-url="selectedArticle.video_url"
            :article-title="selectedArticle.title"
          />
        </div>

        <div class="px-4 pb-4 pt-2 flex flex-wrap items-center justify-between gap-3">
          <div class="flex items-center gap-3 text-sm text-text-secondary">
            <span class="truncate max-w-[320px]">{{ selectedArticle.feed_title }}</span>
            <span>{{ formatDate(selectedArticle.published_at) }}</span>
          </div>
          <div class="flex items-center gap-2">
            <button
              class="px-3 py-1.5 rounded bg-bg-tertiary text-text-primary text-sm hover:bg-bg-secondary transition-colors"
              :title="t('article.action.viewOriginal')"
              @click="openOriginal(selectedArticle)"
            >
              {{ t('article.action.viewOriginal') }}
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.video-gallery-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: clamp(12px, 1.6vw, 20px);
}

.video-gallery-card {
  border-radius: 12px;
  background: transparent;
}

.video-gallery-media {
  aspect-ratio: 16 / 9;
}

.video-viewer-shell {
  max-width: min(96vw, calc(90vh * 16 / 9));
}
</style>
