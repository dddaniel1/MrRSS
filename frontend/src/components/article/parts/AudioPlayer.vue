<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import {
  PhMusicNotes,
  PhSpeakerHigh,
  PhPlay,
  PhPause,
  PhGauge,
  PhSpinner,
  PhRewind,
  PhFastForward,
} from '@phosphor-icons/vue';
import { useI18n } from 'vue-i18n';
import { useGlobalAudioPlayer } from '@/composables/article/useGlobalAudioPlayer';

interface Props {
  audioUrl: string;
  articleTitle: string;
  articleId?: number;
}

const props = defineProps<Props>();

const { t } = useI18n();

const {
  currentSourceUrl,
  isPlaying,
  currentTime,
  duration,
  buffered,
  isLoading,
  playbackSpeed,
  volume,
  toggleArticlePlayback,
  prepareArticleAudio,
  seekToTime,
  cycleSpeed,
  setVolume,
  skipBackward,
  skipForward,
} = useGlobalAudioPlayer();

const isCurrentSource = computed(() => currentSourceUrl.value === props.audioUrl);
const displayIsPlaying = computed(() => isCurrentSource.value && isPlaying.value);
const displayIsLoading = computed(() => isCurrentSource.value && isLoading.value);
const displayCurrentTime = computed(() => (isCurrentSource.value ? currentTime.value : 0));
const displayDuration = computed(() => (isCurrentSource.value ? duration.value : 0));
const displayBuffered = computed(() => (isCurrentSource.value ? buffered.value : 0));

onMounted(() => {
  if (!currentSourceUrl.value) {
    prepareArticleAudio({
      url: props.audioUrl,
      title: props.articleTitle,
      articleId: props.articleId,
    });
  }
});

// Format time in MM:SS format
function formatTime(seconds: number): string {
  if (!isFinite(seconds)) return '0:00';
  const mins = Math.floor(seconds / 60);
  const secs = Math.floor(seconds % 60);
  return `${mins}:${secs.toString().padStart(2, '0')}`;
}

async function handleTogglePlay() {
  await toggleArticlePlayback({
    url: props.audioUrl,
    title: props.articleTitle,
    articleId: props.articleId,
  });
}

// Handle dragging on progress bar
const isDragging = ref(false);
let progressBarRect: DOMRect | null = null;

function onProgressMouseDown(event: MouseEvent) {
  const progressBar = event.currentTarget as HTMLElement;
  isDragging.value = true;
  progressBarRect = progressBar.getBoundingClientRect();

  // Seek to initial position
  const newTime = calculateSeekPosition(event);
  seekToTime(newTime);

  const handleMouseMove = (e: MouseEvent) => {
    if (isDragging.value && progressBarRect) {
      const seekTime = calculateSeekPosition(e);
      seekToTime(seekTime);
    }
  };

  const handleMouseUp = () => {
    isDragging.value = false;
    progressBarRect = null;
    document.removeEventListener('mousemove', handleMouseMove);
    document.removeEventListener('mouseup', handleMouseUp);
  };

  document.addEventListener('mousemove', handleMouseMove);
  document.addEventListener('mouseup', handleMouseUp);
}

// Calculate seek time from mouse event
function calculateSeekPosition(event: MouseEvent): number {
  if (!progressBarRect || !displayDuration.value) return 0;
  const clickX = event.clientX - progressBarRect.left;
  const percentage = Math.max(0, Math.min(1, clickX / progressBarRect.width));
  return percentage * displayDuration.value;
}

// Computed progress percentage
const progressPercentage = computed(() => {
  if (!displayDuration.value) return 0;
  return (displayCurrentTime.value / displayDuration.value) * 100;
});

function onVolumeChange(event: Event) {
  const target = event.target as HTMLInputElement;
  setVolume(parseFloat(target.value));
}

// Extract filename from audio URL
const downloadFilename = computed(() => {
  try {
    const url = new URL(props.audioUrl);
    const pathname = url.pathname;
    const filename = pathname.substring(pathname.lastIndexOf('/') + 1);
    // If filename has no extension or is empty, use article title with .mp3
    if (!filename || !filename.includes('.')) {
      return `${props.articleTitle}.mp3`;
    }
    return filename;
  } catch {
    // Fallback if URL parsing fails
    return `${props.articleTitle}.mp3`;
  }
});
</script>

<template>
  <div class="bg-bg-secondary border border-border rounded-lg p-4 mb-4 sm:mb-6">
    <div class="flex items-center gap-3 mb-3">
      <PhMusicNotes :size="20" class="text-accent flex-shrink-0" />
      <span class="text-sm font-medium text-text-primary">{{
        t('article.audioPlayer.podcastAudio')
      }}</span>
    </div>

    <!-- Custom audio controls -->
    <div class="space-y-3">
      <!-- Progress bar row -->
      <div class="flex items-center gap-3">
        <!-- Skip backward button -->
        <button
          class="flex items-center justify-center w-8 h-8 rounded-full bg-bg-tertiary hover:bg-bg-hover transition-colors flex-shrink-0"
          :title="t('article.audioPlayer.skipBackward')"
          @click="skipBackward"
        >
          <PhRewind :size="16" class="text-text-primary" />
        </button>

        <!-- Play/Pause button -->
        <button
          class="flex items-center justify-center w-10 h-10 rounded-full bg-accent hover:bg-accent/90 transition-colors flex-shrink-0 relative"
          :title="displayIsPlaying ? t('article.audioPlayer.pause') : t('article.audioPlayer.play')"
          @click="handleTogglePlay"
        >
          <PhSpinner v-if="displayIsLoading" :size="20" class="text-white animate-spin" />
          <PhPlay v-else-if="!displayIsPlaying" :size="20" class="text-white ml-0.5" />
          <PhPause v-else :size="20" class="text-white" />
        </button>

        <!-- Skip forward button -->
        <button
          class="flex items-center justify-center w-8 h-8 rounded-full bg-bg-tertiary hover:bg-bg-hover transition-colors flex-shrink-0"
          :title="t('article.audioPlayer.skipForward')"
          @click="skipForward"
        >
          <PhFastForward :size="16" class="text-text-primary" />
        </button>

        <!-- Progress bar -->
        <div class="flex-1 flex items-center gap-2">
          <span class="text-xs text-text-secondary min-w-[40px] text-right">{{
            formatTime(displayCurrentTime)
          }}</span>
          <div
            class="flex-1 h-2 bg-bg-tertiary rounded-full cursor-pointer relative group"
            @mousedown="onProgressMouseDown"
          >
            <!-- Buffered progress -->
            <div
              class="absolute top-0 left-0 h-full bg-bg-hover rounded-full transition-all duration-300"
              :style="{ width: `${Math.min(displayBuffered, 100)}%` }"
            />
            <!-- Played progress -->
            <div
              class="absolute top-0 left-0 h-full bg-accent rounded-full transition-all duration-75"
              :style="{ width: `${progressPercentage}%` }"
            />
            <!-- Draggable thumb (visible on hover and during drag) -->
            <div
              class="absolute top-1/2 -translate-y-1/2 w-3 h-3 bg-accent rounded-full shadow-lg opacity-0 group-hover:opacity-100 transition-opacity duration-200"
              :class="{ 'opacity-100': isDragging }"
              :style="{ left: `calc(${progressPercentage}% - 6px)` }"
            />
            <!-- Loading text indicator -->
            <span
              v-if="displayIsLoading"
              class="absolute left-1/2 -translate-x-1/2 top-1/2 -translate-y-1/2 text-[10px] text-text-secondary font-medium px-2 py-0.5 bg-bg-tertiary/95 rounded-full backdrop-blur-sm whitespace-nowrap z-10"
            >
              {{ t('common.pagination.loading') }}...
            </span>
          </div>
          <span class="text-xs text-text-secondary min-w-[40px]">{{
            formatTime(displayDuration)
          }}</span>
        </div>
      </div>

      <!-- Download and controls row -->
      <div class="flex items-center justify-between pt-3 border-t border-border">
        <!-- Download link -->
        <a
          :href="audioUrl"
          :download="downloadFilename"
          class="text-xs text-accent hover:underline flex items-center gap-1"
          target="_blank"
        >
          {{ t('common.contextMenu.downloadAudio') }}
        </a>

        <!-- Controls -->
        <div class="flex items-center gap-3">
          <!-- Playback speed control -->
          <button
            class="flex items-center gap-1.5 px-2 py-1 rounded-md bg-bg-tertiary hover:bg-bg-tertiary/80 transition-colors text-xs font-medium text-text-primary min-w-[70px]"
            :title="t('article.audioPlayer.playbackSpeed')"
            @click="cycleSpeed"
          >
            <PhGauge :size="12" class="text-text-secondary" />
            <span>{{ playbackSpeed }}x</span>
          </button>

          <!-- Volume control -->
          <div class="flex items-center gap-1.5">
            <PhSpeakerHigh :size="14" class="text-text-secondary flex-shrink-0" />
            <input
              type="range"
              min="0"
              max="1"
              step="0.05"
              :value="volume"
              class="w-20 h-1.5 bg-bg-tertiary rounded-full appearance-none cursor-pointer [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:w-3 [&::-webkit-slider-thumb]:h-3 [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-accent [&::-webkit-slider-thumb]:cursor-pointer [&::-webkit-slider-thumb]:transition-all [&::-webkit-slider-thumb]:hover:scale-125"
              :title="t('article.audioPlayer.volume')"
              @input="onVolumeChange"
            />
            <span class="text-xs text-text-secondary w-[35px] text-right"
              >{{ Math.round(volume * 100) }}%</span
            >
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
