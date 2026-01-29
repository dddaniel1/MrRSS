<script setup lang="ts">
import { ref, computed } from 'vue';
import { PhPlay, PhYoutubeLogo } from '@phosphor-icons/vue';
import { useI18n } from 'vue-i18n';

interface Props {
  videoUrl: string;
  articleTitle: string;
}

const props = defineProps<Props>();

const { t } = useI18n();

const iframeRef = ref<HTMLIFrameElement | HTMLVideoElement | null>(null);
const isLoading = ref(true);

const isYouTube = computed(() => !!getYouTubeId(props.videoUrl));
const isBilibiliIframe = computed(() =>
  props.videoUrl.includes('bilibili.com/blackboard/html5mobileplayer.html') ||
  props.videoUrl.includes('player.bilibili.com')
);
const isIframe = computed(() => isYouTube.value || isBilibiliIframe.value);
const embedUrl = computed(() => {
  if (!isYouTube.value) return props.videoUrl;
  const id = getYouTubeId(props.videoUrl);
  if (!id) return props.videoUrl;
  return `https://www.youtube.com/embed/${id}`;
});

function onLoad() {
  isLoading.value = false;
}

function onError() {
  isLoading.value = false;
  window.showToast(t('article.videoPlayer.videoLoadError'), 'error');
}

// Open video in new tab
function openInNewTab() {
  if (isYouTube.value) {
    const id = getYouTubeId(props.videoUrl);
    if (id) {
      window.open(`https://www.youtube.com/watch?v=${id}`, '_blank');
      return;
    }
  }
  window.open(props.videoUrl, '_blank');
}

function getYouTubeId(url: string): string | null {
  try {
    const parsed = new URL(url);
    if (parsed.hostname.includes('youtu.be')) {
      const id = parsed.pathname.split('/').filter(Boolean)[0];
      return id || null;
    }
    if (parsed.hostname.includes('youtube.com')) {
      if (parsed.pathname.startsWith('/embed/')) {
        const id = parsed.pathname.split('/').filter(Boolean)[1];
        return id || null;
      }
      if (parsed.pathname.startsWith('/shorts/')) {
        const id = parsed.pathname.split('/').filter(Boolean)[1];
        return id || null;
      }
      const id = parsed.searchParams.get('v');
      return id || null;
    }
  } catch {
    if (url.includes('youtu.be/')) {
      const match = url.split('youtu.be/')[1];
      return match ? match.split(/[?&/]/)[0] : null;
    }
  }
  return null;
}

</script>

<template>
  <div class="bg-bg-secondary border border-border rounded-lg overflow-hidden mb-4 sm:mb-6">
    <!-- Header -->
    <div class="flex items-center justify-between p-3 border-b border-border">
      <div class="flex items-center gap-2">
        <PhYoutubeLogo
          v-if="isYouTube"
          :size="20"
          class="text-red-600 flex-shrink-0"
        />
        <span
          v-else-if="isBilibiliIframe"
          class="text-xs font-semibold px-1.5 py-0.5 rounded bg-[#00A1D6]/15 text-[#00A1D6]"
        >
          bilibili
        </span>
        <PhPlay v-else :size="20" class="text-text-secondary flex-shrink-0" />
        <span class="text-sm font-medium text-text-primary">
          {{
            isYouTube
              ? t('article.videoPlayer.youtubeVideo')
              : isBilibiliIframe
                ? t('article.videoPlayer.bilibiliVideo')
                : t('article.videoPlayer.video')
          }}
        </span>
      </div>
      <button
        class="text-xs text-accent hover:underline"
        :title="isYouTube ? t('article.videoPlayer.openInYouTube') : t('article.action.openInBrowser')"
        @click="openInNewTab"
      >
        {{ isYouTube ? t('article.videoPlayer.openInYouTube') : t('article.action.openInBrowser') }}
      </button>
    </div>

    <!-- Video Player -->
    <div class="relative w-full" style="padding-bottom: 56.25%">
      <!-- 16:9 Aspect Ratio -->
      <iframe
        v-if="isIframe"
        ref="iframeRef"
        :src="isYouTube ? embedUrl : videoUrl"
        :title="articleTitle"
        class="absolute top-0 left-0 w-full h-full border-none"
        allow="
          accelerometer;
          autoplay;
          clipboard-write;
          encrypted-media;
          gyroscope;
          picture-in-picture;
          web-share;
        "
        allowfullscreen
        @load="onLoad"
        @error="onError"
      />
      <video
        v-else
        ref="iframeRef"
        class="absolute top-0 left-0 w-full h-full bg-black"
        controls
        :src="embedUrl"
        :title="articleTitle"
        @loadeddata="onLoad"
        @error="onError"
      ></video>

      <!-- Loading indicator -->
      <div
        v-if="isLoading"
        class="absolute inset-0 flex items-center justify-center bg-bg-tertiary"
      >
        <div
          class="animate-spin rounded-full h-12 w-12 border-4 border-accent border-t-transparent"
        ></div>
      </div>
    </div>
  </div>
</template>
