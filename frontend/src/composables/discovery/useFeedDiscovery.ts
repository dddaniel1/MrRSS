import { ref, type Ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { Feed } from '@/types/models';
import type { DiscoveredFeed, FailedCandidate, ProgressCounts, ProgressState } from '@/types/discovery';

export type DiscoveryMode = 'discover' | 'recommend';

export function mapRecommendationErrorCode(
  t: (key: string) => string,
  errorCode: string,
): string {
  switch (errorCode) {
    case 'recommendation_ai_not_configured':
      return t('modal.discovery.recommendationAiNotConfigured');
    case 'recommendation_ai_generation_failed':
      return t('modal.discovery.recommendationAiGenerationFailed');
    case 'recommendation_invalid_candidates':
      return t('modal.discovery.recommendationInvalidCandidates');
    case 'recommendation_validation_failed':
      return t('modal.discovery.recommendationValidationFailed');
    case 'recommendation_validation_timed_out':
      return t('modal.discovery.recommendationValidationTimedOut');
    case 'recommendation_feed_not_found':
      return t('modal.discovery.recommendationFeedNotFound');
    default:
      return t('modal.discovery.discoveryFailed') + ': ' + errorCode;
  }
}

export function useFeedDiscovery(feed: Feed, mode: DiscoveryMode = 'discover') {
  const { t } = useI18n();

  const isDiscovering = ref(false);
  const discoveredFeeds: Ref<DiscoveredFeed[]> = ref([]);
  const failedCandidates: Ref<FailedCandidate[]> = ref([]);
  const errorMessage = ref('');
  const progressMessage = ref('');
  const progressDetail = ref('');
  const progressCounts: Ref<ProgressCounts> = ref({ current: 0, total: 0, found: 0 });
  let pollInterval: ReturnType<typeof setInterval> | null = null;

  function getHostname(url: string): string {
    try {
      return new URL(url).hostname;
    } catch {
      return url;
    }
  }

  function getRecommendationErrorMessage(errorCode: string): string {
    return mapRecommendationErrorCode((key) => t(key), errorCode);
  }

  async function startDiscovery() {
    isDiscovering.value = true;
    errorMessage.value = '';
    discoveredFeeds.value = [];
    failedCandidates.value = [];
    progressMessage.value =
      mode === 'recommend' ? t('modal.discovery.analyzingFeed') : t('modal.discovery.fetchingHomepage');
    progressDetail.value = '';
    progressCounts.value = { current: 0, total: 0, found: 0 };

    // Clear any existing poll interval
    if (pollInterval) {
      clearInterval(pollInterval);
      pollInterval = null;
    }

    try {
      // Validate feed ID
      if (!feed?.id) {
        throw new Error('Invalid feed ID');
      }

      const clearEndpoint =
        mode === 'recommend' ? '/api/feeds/recommend/clear' : '/api/feeds/discover/clear';
      const startEndpoint =
        mode === 'recommend' ? '/api/feeds/recommend/start' : '/api/feeds/discover/start';
      const progressEndpoint =
        mode === 'recommend' ? '/api/feeds/recommend/progress' : '/api/feeds/discover/progress';

      // Clear any previous state
      await fetch(clearEndpoint, { method: 'POST' });

      // Start discovery/recommendation in background
      const startResponse = await fetch(startEndpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ feed_id: feed.id }),
      });

      if (!startResponse.ok) {
        const errorText = await startResponse.text();
        throw new Error(errorText || 'Failed to start discovery');
      }

      // Start polling for progress
      pollInterval = setInterval(async () => {
        try {
          const progressResponse = await fetch(progressEndpoint);
          if (!progressResponse.ok) {
            throw new Error('Failed to get progress');
          }

          const state = (await progressResponse.json()) as ProgressState;

          // Update progress display
          if (state.progress) {
            const progress = state.progress;
            switch (progress.stage) {
              case 'fetching_homepage':
                progressMessage.value = t('modal.discovery.fetchingHomepage');
                progressDetail.value = progress.detail ? getHostname(progress.detail) : '';
                break;
              case 'finding_friend_links':
                progressMessage.value = t('modal.discovery.searchingFriendLinks');
                progressDetail.value = progress.detail ? getHostname(progress.detail) : '';
                break;
              case 'fetching_friend_page':
                progressMessage.value = t('modal.discovery.fetchingFriendPage');
                progressDetail.value = progress.detail ? getHostname(progress.detail) : '';
                break;
              case 'found_links':
                progressMessage.value = t('modal.discovery.foundPotentialLinks', {
                  count: progress.total,
                });
                progressDetail.value = '';
                progressCounts.value.total = progress.total || 0;
                break;
              case 'checking_rss':
                progressMessage.value = t('modal.discovery.checkingRssFeed');
                progressDetail.value = progress.detail ? getHostname(progress.detail) : '';
                progressCounts.value.current = progress.current || 0;
                progressCounts.value.total = progress.total || 0;
                progressCounts.value.found = progress.found_count || 0;
                break;
              case 'analyzing_feed':
                progressMessage.value = t('modal.discovery.analyzingFeed');
                progressDetail.value = progress.detail || '';
                break;
              case 'generating_candidates':
                progressMessage.value = t('modal.discovery.findingSimilarFeeds');
                progressDetail.value = progress.detail ? getHostname(progress.detail) : '';
                progressCounts.value.current = progress.current || 0;
                progressCounts.value.total = progress.total || 0;
                progressCounts.value.found = progress.found_count || 0;
                break;
              case 'validating_candidates':
                progressMessage.value = t('modal.discovery.aiRankingFeeds');
                progressDetail.value = progress.detail ? getHostname(progress.detail) : '';
                progressCounts.value.current = progress.current || 0;
                progressCounts.value.total = progress.total || 0;
                progressCounts.value.found = progress.found_count || 0;
                break;
              default:
                progressMessage.value = progress.message || t('modal.discovery.discovering');
                progressDetail.value = progress.detail ? getHostname(progress.detail) : '';
            }
          }

          // Check if complete
          if (state.is_complete) {
            if (pollInterval !== null) {
              clearInterval(pollInterval);
              pollInterval = null;
            }

            if (state.error) {
              errorMessage.value =
                mode === 'recommend'
                  ? getRecommendationErrorMessage(state.error)
                  : t('modal.discovery.discoveryFailed') + ': ' + state.error;
            } else {
              discoveredFeeds.value = state.feeds || [];
              failedCandidates.value = state.failed_candidates || [];
              if (discoveredFeeds.value.length === 0 && failedCandidates.value.length === 0) {
                errorMessage.value =
                  mode === 'recommend'
                    ? t('modal.discovery.noRecommendedFeeds')
                    : t('modal.discovery.noFriendLinksFound');
              }
            }

            isDiscovering.value = false;
            progressMessage.value = '';
            progressDetail.value = '';

            // Clear the server state
            await fetch(clearEndpoint, { method: 'POST' });
          }
        } catch (pollError) {
          console.error('Polling error:', pollError);
          // Don't stop polling on transient errors
        }
      }, 500); // Poll every 500ms
    } catch (error) {
      console.error('Discovery error:', error);
      errorMessage.value = t('modal.discovery.discoveryFailed') + ': ' + (error as Error).message;
      isDiscovering.value = false;
      progressMessage.value = '';
      progressDetail.value = '';
      if (pollInterval) {
        clearInterval(pollInterval);
        pollInterval = null;
      }
    }
  }

  function cleanup() {
    if (pollInterval) {
      clearInterval(pollInterval);
      pollInterval = null;
    }
    // Clear discovery/recommendation state on server
    const clearEndpoint =
      mode === 'recommend' ? '/api/feeds/recommend/clear' : '/api/feeds/discover/clear';
    fetch(clearEndpoint, { method: 'POST' }).catch(() => {});
  }

  return {
    isDiscovering,
    discoveredFeeds,
    failedCandidates,
    errorMessage,
    progressMessage,
    progressDetail,
    progressCounts,
    startDiscovery,
    cleanup,
  };
}
