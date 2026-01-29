/**
 * Composable for feed management operations in settings
 */
import { useI18n } from 'vue-i18n';
import { useAppStore } from '@/stores/app';
import type { Feed } from '@/types/models';

export function useFeedManagement() {
  const { t } = useI18n();
  const store = useAppStore();

  /**
   * Import OPML file using dialog
   */
  async function handleImportOPML() {
    try {
      console.log('Starting OPML import dialog...');
      const response = await fetch('/api/opml/import-dialog', {
        method: 'POST',
      });

      if (!response.ok) {
        // Handle HTTP error responses
        const errorData = await response.json().catch(() => ({ error: 'Unknown error' }));
        throw new Error(errorData.error || 'Import failed');
      }

      const result = await response.json();

      if (result.status === 'cancelled') {
        console.log('OPML import cancelled by user');
        return;
      }

      if (result.status === 'success') {
        console.log('OPML import successful:', result);
        window.showToast(t('modal.feed.feedAddedSuccess'), 'success');
        store.fetchFeeds();
        // Start polling for progress as the backend is now fetching articles for imported feeds
        store.pollProgress();
      } else {
        console.error('OPML import failed:', result);
        window.showToast(t('common.errors.addingFeed'), 'error');
      }
    } catch (error) {
      console.error('OPML import network error:', error);
      window.showToast(t('common.errors.addingFeed'), 'error');
    }
  }

  /**
   * Export OPML file using dialog
   */
  async function handleExportOPML() {
    try {
      console.log('Starting OPML export dialog...');
      const response = await fetch('/api/opml/export-dialog', {
        method: 'POST',
      });

      if (!response.ok) {
        // Handle HTTP error responses
        const errorData = await response.json().catch(() => ({ error: 'Unknown error' }));
        throw new Error(errorData.error || 'Export failed');
      }

      const result = await response.json();

      if (result.status === 'cancelled') {
        console.log('OPML export cancelled by user');
        return;
      }

      if (result.status === 'success') {
        console.log('OPML export successful:', result);
        window.showToast(t('modal.opml.exportSuccess'), 'success');
      } else {
        console.error('OPML export failed:', result);
        window.showToast(t('common.errors.unknownError'), 'error');
      }
    } catch (error) {
      console.error('OPML export network error:', error);
      window.showToast(t('common.errors.unknownError'), 'error');
    }
  }

  /**
   * Clean up old articles from database
   */
  async function handleCleanupDatabase() {
    const confirmed = await window.showConfirm({
      title: t('setting.database.cleanDatabaseTitle'),
      message: t('setting.database.cleanDatabaseMessage'),
      confirmText: t('setting.database.clean'),
      cancelText: t('common.cancel'),
      isDanger: true,
    });
    if (!confirmed) return;

    try {
      const res = await fetch('/api/articles/cleanup', { method: 'POST' });
      if (res.ok) {
        const result = await res.json();
        window.showToast(t('modal.feed.articlesDeleted', { count: result.deleted }), 'success');
        store.fetchArticles();
      } else {
        window.showToast(t('common.errors.cleaningDatabase'), 'error');
      }
    } catch (e) {
      console.error('Error cleaning database:', e);
      window.showToast(t('common.errors.cleaningDatabase'), 'error');
    }
  }

  /**
   * Add new feed
   */
  function handleAddFeed() {
    window.dispatchEvent(new CustomEvent('show-add-feed'));
  }

  /**
   * Edit existing feed
   */
  function handleEditFeed(feed: Feed) {
    window.dispatchEvent(new CustomEvent('show-edit-feed', { detail: feed }));
  }

  /**
   * Delete a single feed
   */
  async function handleDeleteFeed(id: number) {
    const confirmed = await window.showConfirm({
      title: t('modal.feed.deleteFeedTitle'),
      message: t('modal.feed.deleteFeedMessage'),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      isDanger: true,
    });
    if (!confirmed) return;

    await fetch(`/api/feeds/delete?id=${id}`, { method: 'POST' });
    store.fetchFeeds();
    window.showToast(t('modal.feed.feedDeletedSuccess'), 'success');
  }

  /**
   * Delete multiple feeds
   */
  async function handleBatchDelete(selectedIds: number[]) {
    const confirmed = await window.showConfirm({
      title: t('modal.feed.deleteMultipleFeedsTitle'),
      message: t('modal.feed.deleteMultipleFeedsMessage', { count: selectedIds.length }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      isDanger: true,
    });
    if (!confirmed) return;

    const promises = selectedIds.map((id: number) =>
      fetch(`/api/feeds/delete?id=${id}`, { method: 'POST' })
    );
    await Promise.all(promises);
    store.fetchFeeds();
    window.showToast(t('modal.feed.feedsDeletedSuccess'), 'success');
  }

  /**
   * Get categories excluding FreshRSS-only categories
   */
  function getNonFreshRSSCategories(): string[] {
    if (!store.feeds) return [];

    const categoryFeedsMap = new Map<string, boolean>();

    // Build a map of category -> whether it has non-FreshRSS feeds
    store.feeds.forEach((feed) => {
      if (feed.category && feed.category.trim() !== '') {
        if (!categoryFeedsMap.has(feed.category)) {
          categoryFeedsMap.set(feed.category, !feed.is_freshrss_source);
        } else {
          // Update if we find a non-FreshRSS feed in this category
          if (!feed.is_freshrss_source) {
            categoryFeedsMap.set(feed.category, true);
          }
        }
      }
    });

    // Filter out categories where all feeds are from FreshRSS
    // or category name ends with " (FreshRSS)" or matches pattern " (FreshRSS \d+)$"
    const categories = Array.from(categoryFeedsMap.entries())
      .filter(([_, hasNonFreshRSS]) => hasNonFreshRSS)
      .filter(([categoryName]) => {
        return !categoryName.endsWith(' (FreshRSS)') && !categoryName.match(/ \(FreshRSS \d+\)$/);
      })
      .map(([categoryName]) => categoryName)
      .sort();

    return categories;
  }

  /**
   * Enable image mode for multiple feeds
   */
  async function handleBatchEnableImageMode(selectedIds: number[]) {
    if (selectedIds.length === 0) return;

    try {
      const response = await fetch('/api/feeds/bulk-update-image-mode', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          feed_ids: selectedIds,
          is_image_mode: true,
        }),
      });

      if (!response.ok) {
        window.showToast(t('common.errors.unknownError'), 'error');
        return;
      }

      const result = await response.json();
      store.fetchFeeds();
      window.showToast(t('modal.feed.feedsUpdatedSuccess', { count: result.updated }), 'success');
    } catch (error) {
      console.error('Error enabling image mode:', error);
      window.showToast(t('common.errors.unknownError'), 'error');
    }
  }

  /**
   * Disable image mode for multiple feeds
   */
  async function handleBatchDisableImageMode(selectedIds: number[]) {
    if (selectedIds.length === 0) return;

    try {
      const response = await fetch('/api/feeds/bulk-update-image-mode', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          feed_ids: selectedIds,
          is_image_mode: false,
        }),
      });

      if (!response.ok) {
        window.showToast(t('common.errors.unknownError'), 'error');
        return;
      }

      const result = await response.json();
      store.fetchFeeds();
      window.showToast(t('modal.feed.feedsUpdatedSuccess', { count: result.updated }), 'success');
    } catch (error) {
      console.error('Error disabling image mode:', error);
      window.showToast(t('common.errors.unknownError'), 'error');
    }
  }

  /**
   * Toggle video mode for multiple feeds
   */
  async function handleBatchUpdateVideoMode(selectedIds: number[], isVideoMode: boolean) {
    if (selectedIds.length === 0) return;

    try {
      const response = await fetch('/api/feeds/bulk-update-video-mode', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          feed_ids: selectedIds,
          is_video_mode: isVideoMode,
        }),
      });

      if (!response.ok) {
        window.showToast(t('common.errors.unknownError'), 'error');
        return;
      }

      const result = await response.json();
      store.fetchFeeds();
      window.showToast(t('modal.feed.feedsUpdatedSuccess', { count: result.updated }), 'success');
    } catch (error) {
      console.error('Error updating video mode:', error);
      window.showToast(t('common.errors.unknownError'), 'error');
    }
  }


  /**
   * Move multiple feeds to a new category
   */
  async function handleBatchMove(selectedIds: number[]) {
    if (!store.feeds) return;

    const categories = getNonFreshRSSCategories();

    const newCategory = await window.showInput({
      title: t('common.action.moveFeeds'),
      message: t('modal.feed.enterCategoryName'),
      placeholder: t('modal.feed.categoryPlaceholder'),
      confirmText: t('common.action.move'),
      cancelText: t('common.action.cancel'),
      suggestions: categories,
    });
    if (newCategory === null) return;

    const promises = selectedIds.map((id: number) => {
      const feed = store.feeds.find((f) => f.id === id);
      if (!feed) return Promise.resolve();
      return fetch('/api/feeds/update', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          id: feed.id,
          title: feed.title,
          url: feed.url,
          category: newCategory,
          is_image_mode: feed.is_image_mode,
          is_video_mode: feed.is_video_mode,
          website_url: feed.website_url,
          image_url: feed.image_url,
          script_path: feed.script_path,
          hide_from_timeline: feed.hide_from_timeline,
          proxy_url: feed.proxy_url,
          proxy_enabled: feed.proxy_enabled,
          refresh_interval: feed.refresh_interval,
          type: feed.type,
          xpath_item: feed.xpath_item,
          xpath_item_title: feed.xpath_item_title,
          xpath_item_content: feed.xpath_item_content,
          xpath_item_uri: feed.xpath_item_uri,
          xpath_item_author: feed.xpath_item_author,
          xpath_item_timestamp: feed.xpath_item_timestamp,
          xpath_item_time_format: feed.xpath_item_time_format,
          xpath_item_thumbnail: feed.xpath_item_thumbnail,
          xpath_item_categories: feed.xpath_item_categories,
          xpath_item_uid: feed.xpath_item_uid,
          article_view_mode: feed.article_view_mode,
          auto_expand_content: feed.auto_expand_content,
        }),
      });
    });

    await Promise.all(promises);
    store.fetchFeeds();
    window.showToast(t('modal.feed.feedsMovedSuccess'), 'success');
  }

  return {
    handleImportOPML,
    handleExportOPML,
    handleCleanupDatabase,
    handleAddFeed,
    handleEditFeed,
    handleDeleteFeed,
    handleBatchDelete,
    handleBatchMove,
    handleBatchEnableImageMode,
    handleBatchDisableImageMode,
    handleBatchUpdateVideoMode,
  };
}
