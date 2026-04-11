<script setup lang="ts">
import { watch, onMounted, onUnmounted, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { PhX } from '@phosphor-icons/vue';
import type { Feed } from '@/types/models';
import DiscoveredFeedItem from './DiscoveredFeedItem.vue';
import DiscoveryProgress from './DiscoveryProgress.vue';
import { useModalClose } from '@/composables/ui/useModalClose';
import { useFeedDiscovery } from '@/composables/discovery/useFeedDiscovery';
import { useFeedSubscription } from '@/composables/discovery/useFeedSubscription';

const { t } = useI18n();

// Modal close handling
useModalClose(() => close());

interface Props {
  feed: Feed;
  show: boolean;
  mode?: 'discover' | 'recommend';
}

const props = defineProps<Props>();

const emit = defineEmits<{
  close: [];
}>();

// Use discovery composable
const {
  isDiscovering,
  discoveredFeeds,
  failedCandidates,
  errorMessage,
  progressMessage,
  progressDetail,
  progressCounts,
  startDiscovery,
  cleanup: cleanupDiscovery,
} = useFeedDiscovery(props.feed, props.mode || 'discover');

// Use subscription composable
const {
  selectedFeeds,
  isSubscribing,
  hasSelection,
  allSelected,
  toggleFeedSelection,
  selectAll,
  subscribeSelected,
} = useFeedSubscription(props.feed, discoveredFeeds);

const modalTitle = computed(() =>
  props.mode === 'recommend' ? t('modal.discovery.recommendFeeds') : t('modal.discovery.discoverFeeds')
);

function close() {
  // Clear polling interval if active
  cleanupDiscovery();
  emit('close');
}

// Auto-start discovery when component is mounted
onMounted(() => {
  if (props.show) {
    startDiscovery();
  }
});

// Watch for modal opening and trigger discovery (for when modal is reused)
watch(
  () => props.show,
  (newShow, oldShow) => {
    if (newShow && !oldShow) {
      startDiscovery();
    }
  }
);

// Cleanup on unmount
onUnmounted(() => {
  cleanupDiscovery();
});
</script>

<template>
  <div
    v-if="show"
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm p-2 sm:p-4"
    data-modal-open="true"
    style="will-change: transform; transform: translateZ(0)"
  >
    <div
      class="bg-bg-primary w-full max-w-4xl h-full sm:h-auto sm:max-h-[90vh] rounded-none sm:rounded-2xl shadow-2xl border border-border flex flex-col"
    >
      <!-- Header -->
      <div
        class="flex justify-between items-center p-4 sm:p-6 border-b border-border bg-gradient-to-r from-accent/5 to-transparent shrink-0"
      >
        <div class="min-w-0 flex-1">
          <h2 class="text-base sm:text-xl font-bold text-text-primary">
            {{ modalTitle }}
          </h2>
          <p class="text-xs sm:text-sm text-text-secondary mt-1 truncate">
            {{ t('modal.filter.fromFeed') }}: {{ feed.title }}
          </p>
        </div>
        <button
          class="p-1.5 sm:p-2 hover:bg-bg-tertiary rounded-lg transition-colors shrink-0 ml-2"
          @click="close"
        >
          <PhX :size="20" class="sm:w-6 sm:h-6 text-text-secondary" />
        </button>
      </div>

      <!-- Content -->
      <div class="flex-1 overflow-y-auto p-4 sm:p-6 scroll-smooth">
        <!-- Loading State -->
        <DiscoveryProgress
          v-if="isDiscovering"
          :progress-message="progressMessage"
          :progress-detail="progressDetail"
          :progress-counts="progressCounts"
        />

        <!-- Error State -->
        <div
          v-else-if="errorMessage"
          class="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-3 sm:p-4 text-red-600 dark:text-red-400 text-sm sm:text-base"
        >
          {{ errorMessage }}
        </div>

        <!-- Results -->
        <div v-else-if="discoveredFeeds.length > 0 || failedCandidates.length > 0">
          <div
            v-if="discoveredFeeds.length > 0"
            class="mb-3 sm:mb-4 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2 bg-bg-secondary rounded-lg p-2 sm:p-3"
          >
            <p class="text-xs sm:text-sm font-medium text-text-primary">
              {{ t('modal.discovery.foundFeeds', { count: discoveredFeeds.length }) }}
            </p>
            <button
              class="text-xs sm:text-sm text-accent hover:text-accent-hover font-medium px-2 sm:px-3 py-1 rounded hover:bg-accent/10 transition-colors"
              @click="selectAll"
            >
              {{ allSelected ? t('common.action.deselectAll') : t('common.search.selectAll') }}
            </button>
          </div>

          <div class="space-y-2 sm:space-y-3">
            <DiscoveredFeedItem
              v-for="(discoveredFeed, index) in discoveredFeeds"
              :key="index"
              :feed="discoveredFeed"
              :is-selected="selectedFeeds.has(index)"
              @toggle="toggleFeedSelection(index)"
            />
          </div>

          <details
            v-if="failedCandidates.length > 0"
            class="mt-4 sm:mt-5 bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-lg"
          >
            <summary
              class="list-none cursor-pointer px-3 sm:px-4 py-2.5 sm:py-3 text-amber-700 dark:text-amber-300 font-medium text-sm flex items-center justify-between"
            >
              <span>
                {{ t('modal.discovery.failedCandidatesCount', { count: failedCandidates.length }) }}
              </span>
              <span class="text-xs text-amber-600 dark:text-amber-400">{{ t('modal.discovery.viewDetails') }}</span>
            </summary>
            <div class="border-t border-amber-200 dark:border-amber-800 px-3 sm:px-4 py-2.5 sm:py-3 space-y-2">
              <div
                v-for="(candidate, index) in failedCandidates"
                :key="`${candidate.url}-${index}`"
                class="text-xs sm:text-sm rounded-md bg-bg-primary/70 border border-border p-2.5"
              >
                <div class="font-medium text-text-primary break-all">{{ candidate.url }}</div>
                <div class="mt-1 text-text-secondary">
                  {{ t('modal.discovery.failedCandidateStage') }}: {{ candidate.stage }}
                </div>
                <div class="text-text-secondary break-all">
                  {{ t('modal.discovery.failedCandidateReason') }}: {{ candidate.reason }}
                </div>
              </div>
            </div>
          </details>
        </div>

        <!-- Initial State (should not be visible as discovery auto-starts) -->
        <div v-else class="text-center py-12 sm:py-16">
          <div
            class="w-12 h-12 sm:w-16 sm:h-16 border-4 border-accent border-t-transparent rounded-full animate-spin mx-auto mb-3 sm:mb-4"
          ></div>
          <p class="text-text-secondary text-base sm:text-lg">
            {{ t('common.pagination.preparing') }}...
          </p>
        </div>
      </div>

      <!-- Footer -->
      <div
        class="flex flex-col-reverse sm:flex-row sm:justify-between items-stretch sm:items-center gap-2 sm:gap-3 p-4 sm:p-6 border-t border-border bg-bg-secondary/50 shrink-0"
      >
        <button class="btn-secondary text-sm sm:text-base" :disabled="isSubscribing" @click="close">
          {{ t('common.cancel') }}
        </button>
        <button
          :disabled="!hasSelection || isSubscribing"
          :class="[
            'btn-primary flex items-center justify-center gap-2 text-sm sm:text-base',
            (!hasSelection || isSubscribing) && 'opacity-50 cursor-not-allowed',
          ]"
          @click="subscribeSelected"
        >
          <div
            v-if="isSubscribing"
            class="w-3 h-3 sm:w-4 sm:h-4 border-2 border-white border-t-transparent rounded-full animate-spin"
          ></div>
          {{ isSubscribing ? t('modal.feed.subscribing') : t('modal.feed.subscribeSelected') }}
          <span
            v-if="hasSelection && !isSubscribing"
            class="bg-white/20 px-1.5 sm:px-2 py-0.5 rounded-full text-xs sm:text-sm"
            >({{ selectedFeeds.size }})</span
          >
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
@reference "../../../style.css";

.btn-primary {
  @apply px-4 sm:px-6 py-2 sm:py-2.5 bg-accent text-white rounded-lg hover:bg-accent-hover transition-all font-medium shadow-sm hover:shadow-md;
}

.btn-secondary {
  @apply px-4 sm:px-6 py-2 sm:py-2.5 bg-bg-tertiary text-text-primary rounded-lg hover:opacity-80 transition-all font-medium;
}
</style>
