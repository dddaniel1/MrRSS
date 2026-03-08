<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch, nextTick } from 'vue';
import { useAppStore } from '@/stores/app';
import { useI18n } from 'vue-i18n';
import type { Article } from '@/types/models';
import {
  PhHeart,
  PhList,
  PhGlobe,
  PhEnvelope,
  PhEnvelopeOpen,
  PhTwitterLogo,
  PhArrowLeft,
  PhBookmarkSimple,
} from '@phosphor-icons/vue';
import { openInBrowser } from '@/utils/browser';
import { getProxiedMediaUrl, isMediaCacheEnabled, proxyImagesInHtml } from '@/utils/mediaProxy';
import { formatDate as formatDateUtil } from '@/utils/date';
import { imageCache } from '@/utils/imageCache';
import ArticleContent from './ArticleContent.vue';
import ImageViewer from '../common/ImageViewer.vue';

const store = useAppStore();
const { t, locale } = useI18n();

interface Props {
  isSidebarOpen?: boolean;
}

defineProps<Props>();

const emit = defineEmits<{
  toggleSidebar: [];
}>();

const ITEMS_PER_PAGE = 30;
const SCROLL_THRESHOLD_PX = 500;

const articles = ref<Article[]>([]);
const isLoading = ref(false);
const page = ref(1);
const hasMore = ref(true);
const containerRef = ref<HTMLElement | null>(null);
const mediaCacheEnabled = ref(false);
const articleImagesCache = ref<Map<number, string[]>>(new Map());
const feedUrlCache = ref<Map<number, string>>(new Map());

const feedId = computed(() => store.currentFeedId);
const category = computed(() => store.currentCategory);

const sortedArticles = computed(() =>
  [...articles.value].sort((a, b) => {
    return new Date(b.published_at).getTime() - new Date(a.published_at).getTime();
  })
);

type TimelineArticle = Article & {
  description?: string;
  translated_description?: string;
  content?: string;
  translated_content?: string;
};

function extractPlainText(input?: string): string {
  if (!input) return '';
  return input
    .replace(/<[^>]*>/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
}

function getArticleExcerpt(article: Article): string {
  const timelineArticle = article as TimelineArticle;
  const source =
    timelineArticle.translated_description ||
    timelineArticle.description ||
    timelineArticle.translated_content ||
    timelineArticle.content ||
    '';

  const plainText = extractPlainText(source);
  if (!plainText) return '';

  const maxLength = 180;
  return plainText.length > maxLength ? `${plainText.slice(0, maxLength)}...` : plainText;
}

function getProxyImageUrl(article: Article, imageUrl: string): string {
  if (!imageUrl) return '';
  const fallbackFeedUrl = feedUrlCache.value.get(article.id);
  const referer = article.url || fallbackFeedUrl;
  const proxied = mediaCacheEnabled.value
    ? getProxiedMediaUrl(imageUrl, referer)
    : imageUrl;
  return imageCache.getImageUrl(proxied);
}

function getFeedInitial(feedTitle?: string): string {
  if (!feedTitle) return '?';
  return feedTitle.charAt(0).toUpperCase();
}

function getFeedIconUrl(article: Article): string {
  const feed = store.feedMap.get(article.feed_id);
  const iconUrl = feed?.image_url;
  if (!iconUrl) return '';
  if (!mediaCacheEnabled.value) return iconUrl;
  return getProxiedMediaUrl(iconUrl, feed?.url || article.url);
}

function getDisplayImages(article: Article): string[] {
  const cached = articleImagesCache.value.get(article.id);
  if (cached && cached.length > 0) return cached;
  // Fallback to single image_url while loading
  if (article.image_url) return [article.image_url];
  return [];
}

async function fetchArticleImages(article: Article): Promise<void> {
  // Skip if already cached
  if (articleImagesCache.value.has(article.id)) return;
  try {
    const res = await fetch(`/api/articles/extract-images?id=${article.id}`);
    if (res.ok) {
      const data = await res.json();
      if (data.images && Array.isArray(data.images) && data.images.length > 0) {
        if (data.feed_url) {
          const nextFeed = new Map(feedUrlCache.value);
          nextFeed.set(article.id, data.feed_url);
          feedUrlCache.value = nextFeed;
        }
        const nextImages = new Map(articleImagesCache.value);
        nextImages.set(article.id, data.images);
        articleImagesCache.value = nextImages;
        return;
      }
    }
  } catch (e) {
    // Silent fallback
  }
  // Cache fallback so we don't re-fetch
  const fallback = article.image_url ? [article.image_url] : [];
  const nextImages = new Map(articleImagesCache.value);
  nextImages.set(article.id, fallback);
  articleImagesCache.value = nextImages;
}

async function fetchAllArticleImages(articleList: Article[]): Promise<void> {
  const BATCH_SIZE = 5;
  for (let i = 0; i < articleList.length; i += BATCH_SIZE) {
    const batch = articleList.slice(i, i + BATCH_SIZE);
    await Promise.all(batch.map((a) => fetchArticleImages(a)));
  }
}

function formatDateWithI18n(dateStr: string): string {
  return formatDateUtil(dateStr, locale.value, t);
}

async function fetchArticles(loadMore = false) {
  if (isLoading.value) return;
  isLoading.value = true;

  try {
    let url = `/api/articles?page=${page.value}&limit=${ITEMS_PER_PAGE}&filter=timeline`;
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
        hasMore.value = data.length >= ITEMS_PER_PAGE;
        // Fetch all images for newly loaded articles (non-blocking)
        fetchAllArticleImages(data);
      }
    }
  } catch (e) {
    console.error('Failed to load timeline articles:', e);
  } finally {
    isLoading.value = false;
  }
}

function handleScroll() {
  if (!containerRef.value || isLoading.value || !hasMore.value) return;
  const { scrollTop, clientHeight, scrollHeight } = containerRef.value;
  if (scrollTop + clientHeight >= scrollHeight - SCROLL_THRESHOLD_PX) {
    page.value += 1;
    fetchArticles(true);
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
    console.error('Failed to mark as read:', e);
  }
}

async function toggleReadStatus(article: Article) {
  const newState = !article.is_read;
  try {
    const res = await fetch(`/api/articles/read?id=${article.id}&read=${newState}`, { method: 'POST' });
    if (res.ok) {
      article.is_read = newState;
      await store.fetchUnreadCounts();
      await store.fetchFilterCounts();
    }
  } catch (e) {
    console.error('Failed to toggle read status:', e);
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

async function toggleReadLater(article: Article, event?: Event) {
  if (event) event.stopPropagation();
  try {
    const res = await fetch(`/api/articles/toggle-read-later?id=${article.id}`, { method: 'POST' });
    if (res.ok) {
      article.is_read_later = !article.is_read_later;
      await store.fetchFilterCounts();
    }
  } catch (e) {
    console.error('Failed to toggle read later:', e);
  }
}

function openOriginal(article: Article, event?: Event) {
  if (event) event.stopPropagation();
  openInBrowser(article.url);
}

// ── Detail view state ──
const selectedArticle = ref<Article | null>(null);
const detailArticleContent = ref('');
const isLoadingDetailContent = ref(false);
const imageViewerSrc = ref<string | null>(null);
const imageViewerAlt = ref('');
const imageViewerImages = ref<string[]>([]);
const imageViewerInitialIndex = ref(0);
const detailContainerRef = ref<HTMLElement | null>(null);

async function fetchDetailContent(article: Article) {
  isLoadingDetailContent.value = true;
  detailArticleContent.value = '';
  try {
    const res = await fetch(`/api/articles/content?id=${article.id}`);
    if (res.ok) {
      const data = await res.json();
      let content = data.content || '';
      if (mediaCacheEnabled.value && content) {
        const articleBaseUrl = article.url || data.feed_url;
        content = proxyImagesInHtml(content, articleBaseUrl);
      }
      detailArticleContent.value = content;
    }
  } catch (e) {
    console.error('Failed to fetch article content:', e);
  } finally {
    isLoadingDetailContent.value = false;
  }
}

function handleCardClick(article: Article) {
  if (!article.is_read) {
    markAsRead(article);
  }
  selectedArticle.value = article;
  fetchDetailContent(article);
  // Scroll detail to top on next tick
  nextTick(() => {
    if (detailContainerRef.value) {
      detailContainerRef.value.scrollTop = 0;
    }
  });
}

function closeDetail() {
  selectedArticle.value = null;
  detailArticleContent.value = '';
  isLoadingDetailContent.value = false;
  imageViewerSrc.value = null;
  imageViewerAlt.value = '';
  imageViewerImages.value = [];
  imageViewerInitialIndex.value = 0;
}

function closeImageViewer() {
  imageViewerSrc.value = null;
  imageViewerAlt.value = '';
  imageViewerImages.value = [];
  imageViewerInitialIndex.value = 0;
}

function attachDetailImageListeners() {
  // Unwrap images from links
  const links = document.querySelectorAll<HTMLAnchorElement>('.timeline-detail-content .prose-content a');
  const linksToProcess: HTMLAnchorElement[] = [];
  links.forEach((link) => {
    if (link.querySelectorAll('img').length > 0) {
      linksToProcess.push(link);
    }
  });
  linksToProcess.forEach((link) => {
    if (!link.parentNode) return;
    const fragment = document.createDocumentFragment();
    while (link.firstChild) {
      fragment.appendChild(link.firstChild);
    }
    link.parentNode.replaceChild(fragment, link);
  });

  // Attach click handlers to images
  const images = document.querySelectorAll<HTMLImageElement>('.timeline-detail-content .prose-content img');
  images.forEach((img) => {
    if (!img.parentNode) return;
    if (img.height <= 24 && img.height > 0) return;

    img.style.cursor = 'pointer';
    img.style.pointerEvents = 'auto';

    const newImg = img.cloneNode(true) as HTMLImageElement;
    img.parentNode.replaceChild(newImg, img);
    newImg.style.cursor = 'pointer';
    newImg.style.pointerEvents = 'auto';

    newImg.addEventListener('click', (e: Event) => {
      e.preventDefault();
      e.stopPropagation();
      if (!newImg.src) return;
      const allImages = Array.from(
        document.querySelectorAll<HTMLImageElement>('.timeline-detail-content .prose-content img')
      )
        .filter((i) => !(i.height <= 24 && i.height > 0))
        .map((i) => i.src);
      const clickedIndex = allImages.findIndex((src) => src === newImg.src);
      imageViewerSrc.value = newImg.src;
      imageViewerAlt.value = newImg.alt || '';
      imageViewerImages.value = allImages;
      imageViewerInitialIndex.value = clickedIndex >= 0 ? clickedIndex : 0;
    }, { capture: true });
  });

  // Attach click handlers to text links
  const textLinks = document.querySelectorAll<HTMLAnchorElement>('.timeline-detail-content .prose-content a');
  textLinks.forEach((link) => {
    if (!link.parentNode || link.querySelector('img') || link.dataset.linkHandlerAttached === 'true') return;
    link.dataset.linkHandlerAttached = 'true';
    link.addEventListener('click', (e: Event) => {
      e.preventDefault();
      e.stopPropagation();
      let href = link.getAttribute('href');
      if (href) {
        if (href.startsWith('/') || (!href.startsWith('http://') && !href.startsWith('https://') && !href.startsWith('mailto:') && !href.startsWith('#'))) {
          if (selectedArticle.value?.url) {
            try {
              if (!href.startsWith('/')) {
                href = new URL(href, selectedArticle.value.url).href;
              } else {
                const articleUrl = new URL(selectedArticle.value.url);
                href = `${articleUrl.origin}${href}`;
              }
            } catch { /* ignore */ }
          }
        }
        openInBrowser(href);
      }
    }, { capture: true });
  });
}

// Handle ESC to close detail view
function handleDetailKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && selectedArticle.value) {
    e.preventDefault();
    e.stopPropagation();
    closeDetail();
  }
}

// Detail-level actions (operate on selectedArticle)
async function detailToggleRead() {
  if (!selectedArticle.value) return;
  const newState = !selectedArticle.value.is_read;
  try {
    const res = await fetch(`/api/articles/read?id=${selectedArticle.value.id}&read=${newState}`, { method: 'POST' });
    if (res.ok) {
      selectedArticle.value.is_read = newState;
      await store.fetchUnreadCounts();
      await store.fetchFilterCounts();
    }
  } catch (e) {
    console.error('Failed to toggle read status:', e);
  }
}

async function detailToggleFavorite() {
  if (!selectedArticle.value) return;
  try {
    const res = await fetch(`/api/articles/favorite?id=${selectedArticle.value.id}`, { method: 'POST' });
    if (res.ok) {
      selectedArticle.value.is_favorite = !selectedArticle.value.is_favorite;
      await store.fetchFilterCounts();
    }
  } catch (e) {
    console.error('Failed to toggle favorite:', e);
  }
}

async function detailToggleReadLater() {
  if (!selectedArticle.value) return;
  try {
    const res = await fetch(`/api/articles/toggle-read-later?id=${selectedArticle.value.id}`, { method: 'POST' });
    if (res.ok) {
      selectedArticle.value.is_read_later = !selectedArticle.value.is_read_later;
      await store.fetchFilterCounts();
    }
  } catch (e) {
    console.error('Failed to toggle read later:', e);
  }
}

function detailOpenOriginal() {
  if (selectedArticle.value) openInBrowser(selectedArticle.value.url);
}

watch(feedId, async () => {
  if (selectedArticle.value) {
    closeDetail();
  }
  page.value = 1;
  articles.value = [];
  hasMore.value = true;
  await fetchArticles();
  await nextTick();
});

watch(category, async () => {
  if (selectedArticle.value) {
    closeDetail();
  }
  page.value = 1;
  articles.value = [];
  hasMore.value = true;
  await fetchArticles();
  await nextTick();
});

watch(
  () => store.tempSelection,
  () => {
    if (selectedArticle.value) {
      closeDetail();
    }
  }
);

onMounted(async () => {
  mediaCacheEnabled.value = await isMediaCacheEnabled();
  fetchArticles();
  if (containerRef.value) {
    containerRef.value.addEventListener('scroll', handleScroll);
  }
  window.addEventListener('keydown', handleDetailKeydown);
});

onUnmounted(() => {
  if (containerRef.value) {
    containerRef.value.removeEventListener('scroll', handleScroll);
  }
  window.removeEventListener('keydown', handleDetailKeydown);
});
</script>

<template>
  <div class="flex flex-col flex-1 h-full bg-bg-primary relative">
    <!-- Header -->
    <div class="flex-shrink-0 bg-bg-primary/80 backdrop-blur-sm border-b border-border p-2 sm:p-4 flex items-center gap-3 sticky top-0 z-10">
      <button
        class="p-2 rounded-lg hover:bg-bg-tertiary text-text-primary transition-colors md:hidden"
        :title="t('shortcut.toggle.sidebar')"
        @click="emit('toggleSidebar')"
      >
        <PhList :size="24" />
      </button>
      <div class="flex items-center gap-2 flex-1">
        <h1 class="text-base sm:text-lg font-bold text-text-primary line-height-fixed-32">
          {{ t('sidebar.activity.timeline') }}
        </h1>
      </div>
    </div>

    <!-- Timeline content -->
    <div ref="containerRef" class="flex-1 overflow-y-scroll scroll-smooth">
      <div v-if="articles.length > 0" class="timeline-container mx-auto">
        <div
          v-for="article in sortedArticles"
          :key="article.id"
          class="timeline-card"
          :class="{ 'timeline-card--read': article.is_read }"
          @click="handleCardClick(article)"
        >
          <div class="flex gap-3">
            <!-- Avatar -->
            <div class="shrink-0 pt-0.5">
              <div class="timeline-avatar">
                <img
                  v-if="getFeedIconUrl(article)"
                  :src="getFeedIconUrl(article)"
                  :alt="article.feed_title || 'Feed'"
                  class="timeline-avatar-icon"
                />
                <span v-else class="timeline-avatar-fallback">{{ getFeedInitial(article.feed_title) }}</span>
              </div>
            </div>

            <!-- Content -->
            <div class="flex-1 min-w-0">
              <!-- Header: feed name · author · time -->
              <div class="flex items-center gap-1 text-sm leading-5 mb-0.5">
                <span class="font-bold text-text-primary truncate">
                  {{ article.feed_title }}
                </span>
                <template v-if="article.author && article.author !== article.feed_title">
                  <span class="text-text-secondary shrink-0">·</span>
                  <span class="text-text-secondary truncate max-w-[140px]">
                    {{ article.author }}
                  </span>
                </template>
                <span class="text-text-secondary shrink-0">·</span>
                <span class="text-text-secondary whitespace-nowrap shrink-0">
                  {{ formatDateWithI18n(article.published_at) }}
                </span>
              </div>

              <!-- Title -->
              <div class="mb-1.5">
                <h3
                  v-if="article.translated_title && article.translated_title !== article.title"
                  class="text-[15px] leading-snug text-text-primary font-semibold"
                >
                  {{ article.translated_title }}
                  <span class="text-text-secondary text-xs font-normal italic ml-1">
                    {{ article.title }}
                  </span>
                </h3>
                <h3
                  v-else
                  class="text-[15px] leading-snug text-text-primary font-semibold"
                >
                  {{ article.title }}
                </h3>
              </div>

              <p
                v-if="getArticleExcerpt(article)"
                class="mb-2.5 text-[14px] leading-relaxed text-text-secondary line-clamp-3"
              >
                {{ getArticleExcerpt(article) }}
              </p>

              <!-- Image grid -->
              <div
                v-if="getDisplayImages(article).length > 0"
                class="mb-2.5 rounded-xl overflow-hidden border border-border"
                :class="[
                  getDisplayImages(article).length === 1 ? 'timeline-grid-1' :
                  getDisplayImages(article).length === 2 ? 'timeline-grid-2' :
                  getDisplayImages(article).length <= 4 ? 'timeline-grid-sq' :
                  'timeline-grid-multi'
                ]"
                @click.stop
              >
                <!-- 1 image: full width -->
                <template v-if="getDisplayImages(article).length === 1">
                  <img
                    :src="getProxyImageUrl(article, getDisplayImages(article)[0])"
                    :alt="article.title"
                    class="w-full max-h-[300px] object-contain block bg-bg-secondary"
                    loading="lazy"
                  />
                </template>

                <!-- 2+ images: uniform grid -->
                <template v-else>
                  <div
                    v-for="(img, idx) in getDisplayImages(article).slice(0, 9)"
                    :key="idx"
                    class="relative overflow-hidden"
                  >
                    <img
                      :src="getProxyImageUrl(article, img)"
                      :alt="`${article.title} - ${idx + 1}`"
                      class="object-contain w-full h-full block bg-bg-secondary"
                      loading="lazy"
                    />
                    <!-- +N overlay on last cell -->
                    <div
                      v-if="idx === 8 && getDisplayImages(article).length > 9"
                      class="absolute inset-0 bg-black/50 flex items-center justify-center cursor-pointer"
                      @click.stop="handleCardClick(article)"
                    >
                      <span class="text-white text-xl font-bold">
                        +{{ getDisplayImages(article).length - 9 }}
                      </span>
                    </div>
                  </div>
                </template>
              </div>

              <!-- Action bar -->
              <div class="flex items-center justify-between timeline-actions">
                <!-- Read status -->
                <button
                  class="timeline-action-btn"
                  :class="{ 'text-accent': !article.is_read }"
                  :title="article.is_read ? t('article.action.markAsUnread') : t('article.action.markAsRead')"
                  @click.stop="toggleReadStatus(article)"
                >
                  <PhEnvelopeOpen v-if="article.is_read" :size="18" />
                  <PhEnvelope v-else :size="18" />
                </button>

                <!-- Read later -->
                <button
                  class="timeline-action-btn"
                  :class="{ 'text-blue-500': article.is_read_later }"
                  :title="article.is_read_later ? t('article.action.removeFromReadLater') : t('article.action.addToReadLater')"
                  @click.stop="toggleReadLater(article, $event)"
                >
                  <PhBookmarkSimple
                    :size="18"
                    :weight="article.is_read_later ? 'fill' : 'regular'"
                  />
                </button>

                <!-- Favorite -->
                <button
                  class="timeline-action-btn"
                  :class="{ 'text-red-500': article.is_favorite }"
                  :title="article.is_favorite ? t('article.action.removeFromFavorites') : t('article.action.addToFavorite')"
                  @click.stop="toggleFavorite(article, $event)"
                >
                  <PhHeart
                    :size="18"
                    :weight="article.is_favorite ? 'fill' : 'regular'"
                  />
                </button>

                <!-- Open in browser -->
                <button
                  class="timeline-action-btn"
                  :title="t('article.action.openInBrowser')"
                  @click.stop="openOriginal(article, $event)"
                >
                  <PhGlobe :size="18" />
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Empty state -->
      <div v-else-if="!isLoading" class="flex flex-col items-center justify-center h-full gap-4">
        <PhTwitterLogo :size="64" class="text-text-secondary opacity-50" />
        <p class="text-text-secondary">{{ t('article.content.noArticles') }}</p>
      </div>

      <!-- Loading -->
      <div v-if="isLoading" class="flex justify-center py-8">
        <div class="w-8 h-8 border-4 border-accent border-t-transparent rounded-full animate-spin"></div>
      </div>
    </div>

    <!-- Detail overlay (full-screen within timeline area) -->
    <Transition name="timeline-detail">
      <div
        v-if="selectedArticle"
        class="absolute inset-0 z-20 flex flex-col bg-bg-primary"
      >
        <!-- Detail header -->
        <div class="flex-shrink-0 bg-bg-primary/80 backdrop-blur-sm border-b border-border p-2 sm:p-4 flex items-center gap-3">
          <button
            class="p-2 rounded-lg hover:bg-bg-tertiary text-text-primary transition-colors"
            :title="t('article.navigation.back')"
            @click="closeDetail"
          >
            <PhArrowLeft :size="20" />
          </button>
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-1.5 text-sm">
              <div
                class="timeline-avatar-sm"
              >
                <img
                  v-if="getFeedIconUrl(selectedArticle)"
                  :src="getFeedIconUrl(selectedArticle)"
                  :alt="selectedArticle.feed_title || 'Feed'"
                  class="timeline-avatar-sm-icon"
                />
                <span v-else class="timeline-avatar-fallback">{{ getFeedInitial(selectedArticle.feed_title) }}</span>
              </div>
              <span class="font-bold text-text-primary truncate">
                {{ selectedArticle.feed_title }}
              </span>
              <template v-if="selectedArticle.author && selectedArticle.author !== selectedArticle.feed_title">
                <span class="text-text-secondary shrink-0">·</span>
                <span class="text-text-secondary truncate max-w-[140px]">
                  {{ selectedArticle.author }}
                </span>
              </template>
              <span class="text-text-secondary shrink-0">·</span>
              <span class="text-text-secondary whitespace-nowrap shrink-0">
                {{ formatDateWithI18n(selectedArticle.published_at) }}
              </span>
            </div>
          </div>
          <!-- Detail action buttons -->
          <div class="flex items-center gap-1">
            <button
              class="timeline-action-btn"
              :class="{ 'text-accent': !selectedArticle.is_read }"
              :title="selectedArticle.is_read ? t('article.action.markAsUnread') : t('article.action.markAsRead')"
              @click="detailToggleRead"
            >
              <PhEnvelopeOpen v-if="selectedArticle.is_read" :size="18" />
              <PhEnvelope v-else :size="18" />
            </button>
            <button
              class="timeline-action-btn"
              :class="{ 'text-blue-500': selectedArticle.is_read_later }"
              :title="selectedArticle.is_read_later ? t('article.action.removeFromReadLater') : t('article.action.addToReadLater')"
              @click="detailToggleReadLater"
            >
              <PhBookmarkSimple
                :size="18"
                :weight="selectedArticle.is_read_later ? 'fill' : 'regular'"
              />
            </button>
            <button
              class="timeline-action-btn"
              :class="{ 'text-red-500': selectedArticle.is_favorite }"
              :title="selectedArticle.is_favorite ? t('article.action.removeFromFavorites') : t('article.action.addToFavorite')"
              @click="detailToggleFavorite"
            >
              <PhHeart
                :size="18"
                :weight="selectedArticle.is_favorite ? 'fill' : 'regular'"
              />
            </button>
            <button
              class="timeline-action-btn"
              :title="t('article.action.openInBrowser')"
              @click="detailOpenOriginal"
            >
              <PhGlobe :size="18" />
            </button>
          </div>
        </div>

        <!-- Detail content -->
        <div ref="detailContainerRef" class="timeline-detail-content flex-1 overflow-y-auto">
          <ArticleContent
            :article="selectedArticle"
            :article-content="detailArticleContent"
            :is-loading-content="isLoadingDetailContent"
            :attach-image-event-listeners="attachDetailImageListeners"
            :show-content="true"
          />
        </div>

        <!-- Image Viewer Modal -->
        <ImageViewer
          v-if="imageViewerSrc"
          :src="imageViewerSrc"
          :alt="imageViewerAlt"
          :images="imageViewerImages"
          :initial-index="imageViewerInitialIndex"
          @close="closeImageViewer"
        />
      </div>
    </Transition>
  </div>
</template>

<style scoped>
@reference "../../style.css";

.timeline-container {
  max-width: 720px;
  width: 100%;
  padding: 0.75rem;
}

.timeline-card {
  @apply px-4 py-3.5 border border-border rounded-2xl cursor-pointer transition-all;
  margin-bottom: 0.75rem;
  background: color-mix(in srgb, var(--bg-primary) 88%, white 12%);
  box-shadow: 0 1px 2px rgb(15 23 42 / 0.08);
}

.timeline-card:hover {
  background: color-mix(in srgb, var(--bg-primary) 82%, white 18%);
  box-shadow: 0 8px 24px rgb(15 23 42 / 0.12);
}

.timeline-card--read {
  opacity: 0.7;
}

.timeline-card--read:hover {
  opacity: 0.85;
}

.timeline-avatar {
  @apply w-10 h-10 rounded-full flex items-center justify-center select-none shrink-0 bg-bg-secondary border border-border;
}

.timeline-avatar-icon {
  @apply w-6 h-6 object-cover rounded-full;
}

.timeline-avatar-fallback {
  @apply text-text-secondary text-xs font-semibold;
}

.timeline-actions {
  max-width: 320px;
}

.timeline-action-btn {
  @apply p-1.5 rounded-full text-text-secondary transition-colors;
}

.timeline-action-btn:hover {
  @apply bg-bg-tertiary text-accent;
}

/* Image grids — every cell is the same size */
.timeline-grid-1 {
  /* Single image: container only, img handles sizing */
}

.timeline-grid-2 {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 2px;
  height: 200px;
}

.timeline-grid-sq {
  display: grid;
  grid-template-columns: 1fr 1fr;
  grid-auto-rows: 1fr;
  gap: 2px;
  height: 260px;
}

.timeline-grid-multi {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr;
  grid-auto-rows: 1fr;
  gap: 2px;
  height: 390px;
}

/* Detail view small avatar */
.timeline-avatar-sm {
  @apply w-6 h-6 rounded-full flex items-center justify-center select-none shrink-0 bg-bg-secondary border border-border;
}

.timeline-avatar-sm-icon {
  @apply w-4 h-4 object-cover rounded-full;
}

/* Detail overlay transition */
.timeline-detail-enter-active {
  transition: transform 0.25s ease-out, opacity 0.2s ease-out;
}

.timeline-detail-leave-active {
  transition: transform 0.2s ease-in, opacity 0.15s ease-in;
}

.timeline-detail-enter-from {
  transform: translateX(100%);
  opacity: 0;
}

.timeline-detail-leave-to {
  transform: translateX(100%);
  opacity: 0;
}

/* Smaller screens */
@media (max-width: 1400px) {
  .timeline-avatar {
    @apply w-9 h-9 text-xs;
  }
}

/* Mobile */
@media (max-width: 767px) {
  .timeline-container {
    max-width: 100%;
    padding: 0.5rem;
  }

  .timeline-avatar {
    @apply w-8 h-8 text-xs;
  }
}
</style>
