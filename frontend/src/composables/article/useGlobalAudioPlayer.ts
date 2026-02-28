import { ref } from 'vue';
import { useI18n } from 'vue-i18n';

export interface AudioSourceInfo {
  url: string;
  title?: string;
  articleId?: number;
  feedId?: number;
}

const currentSourceUrl = ref('');
const currentArticleTitle = ref('');
const currentArticleId = ref<number | null>(null);
const currentFeedId = ref<number | null>(null);

const isPlaying = ref(false);
const currentTime = ref(0);
const duration = ref(0);
const buffered = ref(0);
const isLoading = ref(false);
const hasLoadedMetadata = ref(false);

const playbackSpeed = ref(1.0);
const volume = ref(1.0);
const speedOptions = [0.5, 0.75, 1.0, 1.25, 1.5, 1.75, 2.0];
const currentSpeedIndex = ref(2);

let audioElement: HTMLAudioElement | null = null;
let hasInitialized = false;
let loadingTimeout: number | null = null;

function getAudioElement(): HTMLAudioElement {
  if (!audioElement) {
    audioElement = new Audio();
    audioElement.preload = 'metadata';
    audioElement.volume = volume.value;
    audioElement.playbackRate = playbackSpeed.value;
  }
  return audioElement;
}

function showLoading() {
  if (loadingTimeout !== null) {
    clearTimeout(loadingTimeout);
  }
  loadingTimeout = window.setTimeout(() => {
    isLoading.value = true;
  }, 200);
}

function hideLoading() {
  if (loadingTimeout !== null) {
    clearTimeout(loadingTimeout);
    loadingTimeout = null;
  }
  isLoading.value = false;
}

function updateBufferedProgress() {
  const audio = getAudioElement();
  if (!duration.value) {
    buffered.value = 0;
    return;
  }

  try {
    const audioBuffered = audio.buffered;
    if (audioBuffered && audioBuffered.length > 0) {
      const bufferedEnd = audioBuffered.end(audioBuffered.length - 1);
      buffered.value = (bufferedEnd / duration.value) * 100;
    } else {
      buffered.value = 0;
    }
  } catch {
    buffered.value = 0;
  }
}

function setSource(source: AudioSourceInfo) {
  const audio = getAudioElement();
  if (audio.src !== source.url) {
    audio.src = source.url;
    audio.load();
    currentTime.value = 0;
    duration.value = 0;
    buffered.value = 0;
    hasLoadedMetadata.value = false;
  }
  currentSourceUrl.value = source.url;
  currentArticleTitle.value = source.title || '';
  currentArticleId.value = source.articleId ?? null;
  currentFeedId.value = source.feedId ?? null;
}

async function playSource(source: AudioSourceInfo, t: ReturnType<typeof useI18n>['t']) {
  setSource(source);
  showLoading();
  try {
    await getAudioElement().play();
  } catch (err) {
    console.error('[AudioPlayer] Failed to play audio:', err);
    hideLoading();
    window.showToast(t('article.audioPlayer.audioPlaybackError'), 'error');
  }
}

function initAudio(t: ReturnType<typeof useI18n>['t']) {
  if (hasInitialized) return;
  const audio = getAudioElement();

  audio.addEventListener('play', () => {
    isPlaying.value = true;
  });

  audio.addEventListener('pause', () => {
    isPlaying.value = false;
    hideLoading();
  });

  audio.addEventListener('timeupdate', () => {
    currentTime.value = audio.currentTime;
    updateBufferedProgress();
    if (isLoading.value && isPlaying.value && currentTime.value > 0) {
      hideLoading();
    }
  });

  audio.addEventListener('loadedmetadata', () => {
    duration.value = audio.duration;
    hasLoadedMetadata.value = true;
    updateBufferedProgress();
  });

  audio.addEventListener('ended', () => {
    isPlaying.value = false;
    currentTime.value = 0;
    hideLoading();
  });

  audio.addEventListener('waiting', () => {
    if (isPlaying.value) {
      showLoading();
    }
  });

  audio.addEventListener('canplay', () => {
    hasLoadedMetadata.value = true;
    hideLoading();
  });

  audio.addEventListener('playing', () => {
    hideLoading();
  });

  audio.addEventListener('seeking', () => {
    if (!isPlaying.value) return;
    const bufferedRanges = audio.buffered;
    let isBuffered = false;
    if (bufferedRanges.length > 0) {
      for (let i = 0; i < bufferedRanges.length; i++) {
        const start = bufferedRanges.start(i);
        const end = bufferedRanges.end(i);
        if (audio.currentTime >= start && audio.currentTime <= end) {
          isBuffered = true;
          break;
        }
      }
    }
    if (!isBuffered) {
      showLoading();
    }
  });

  audio.addEventListener('seeked', () => {
    updateBufferedProgress();
    if (!isPlaying.value) {
      hideLoading();
    }
  });

  audio.addEventListener('progress', () => {
    updateBufferedProgress();
  });

  hasInitialized = true;
}

export function useGlobalAudioPlayer() {
  const { t } = useI18n();
  initAudio(t);

  async function togglePlay() {
    const audio = getAudioElement();
    if (audio.paused) {
      showLoading();
      try {
        await audio.play();
      } catch (err) {
        console.error('[AudioPlayer] Failed to play audio:', err);
        hideLoading();
        window.showToast(t('article.audioPlayer.audioPlaybackError'), 'error');
      }
    } else {
      audio.pause();
    }
  }

  async function toggleArticlePlayback(source: AudioSourceInfo) {
    if (currentSourceUrl.value === source.url) {
      await togglePlay();
      return;
    }
    await playSource(source, t);
  }

  async function playArticleAudio(source: AudioSourceInfo) {
    await playSource(source, t);
  }

  function prepareArticleAudio(source: AudioSourceInfo) {
    if (!currentSourceUrl.value) {
      setSource(source);
    }
  }

  function seekToTime(newTime: number) {
    const audio = getAudioElement();
    const bufferedRanges = audio.buffered;
    let isBuffered = false;
    if (bufferedRanges.length > 0) {
      for (let i = 0; i < bufferedRanges.length; i++) {
        const start = bufferedRanges.start(i);
        const end = bufferedRanges.end(i);
        if (newTime >= start && newTime <= end) {
          isBuffered = true;
          break;
        }
      }
    }
    if (!isBuffered && isPlaying.value) {
      showLoading();
    }
    audio.currentTime = newTime;
  }

  function cycleSpeed() {
    currentSpeedIndex.value = (currentSpeedIndex.value + 1) % speedOptions.length;
    playbackSpeed.value = speedOptions[currentSpeedIndex.value];
    getAudioElement().playbackRate = playbackSpeed.value;
  }

  function setVolume(newVolume: number) {
    volume.value = newVolume;
    getAudioElement().volume = volume.value;
  }

  function skipBackward() {
    const audio = getAudioElement();
    audio.currentTime = Math.max(0, audio.currentTime - 10);
  }

  function skipForward() {
    const audio = getAudioElement();
    audio.currentTime = Math.min(duration.value, audio.currentTime + 10);
  }

  function closePlayer() {
    const audio = getAudioElement();
    audio.pause();
    audio.removeAttribute('src');
    audio.load();

    currentSourceUrl.value = '';
    currentArticleTitle.value = '';
    currentArticleId.value = null;
    currentFeedId.value = null;
    currentTime.value = 0;
    duration.value = 0;
    buffered.value = 0;
    hasLoadedMetadata.value = false;
    isPlaying.value = false;
    hideLoading();
  }

  return {
    currentSourceUrl,
    currentArticleTitle,
    currentArticleId,
    currentFeedId,
    isPlaying,
    currentTime,
    duration,
    buffered,
    isLoading,
    hasLoadedMetadata,
    playbackSpeed,
    volume,
    togglePlay,
    toggleArticlePlayback,
    playArticleAudio,
    prepareArticleAudio,
    seekToTime,
    cycleSpeed,
    setVolume,
    skipBackward,
    skipForward,
    closePlayer,
  };
}
