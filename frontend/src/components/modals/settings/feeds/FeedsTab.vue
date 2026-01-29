<script setup lang="ts">
import { computed } from 'vue';
import DataManagementSettings from './DataManagementSettings.vue';
import FeedManagementSettings from './FeedManagementSettings.vue';
import DiscoverySettings from './DiscoverySettings.vue';
import type { Feed } from '@/types/models';
import type { SettingsData } from '@/types/settings';
import { useSettingsAutoSave } from '@/composables/core/useSettingsAutoSave';

interface Props {
  settings: SettingsData;
}

const props = defineProps<Props>();

const emit = defineEmits<{
  'import-opml': [];
  'export-opml': [];
  'cleanup-database': [];
  'add-feed': [];
  'edit-feed': [feed: Feed];
  'delete-feed': [id: number];
  'batch-delete': [ids: number[]];
  'batch-move': [ids: number[]];
  'batch-enable-image-mode': [ids: number[]];
  'batch-disable-image-mode': [ids: number[]];
  'batch-update-video-mode': [payload: { ids: number[]; enabled: boolean }];
  'discover-all': [];
  'update:settings': [settings: SettingsData];
  'select-feed': [feedId: number];
}>();

// Create a computed ref that returns the settings object
// This ensures reactivity while allowing modifications
const settingsRef = computed(() => props.settings);

// Use composable for auto-save with reactivity
useSettingsAutoSave(settingsRef);

// Event handlers that pass through to parent
function handleImportOPML() {
  emit('import-opml');
}

function handleExportOPML() {
  emit('export-opml');
}

function handleCleanupDatabase() {
  emit('cleanup-database');
}

function handleDiscoverAll() {
  emit('discover-all');
}

function handleAddFeed() {
  emit('add-feed');
}

function handleEditFeed(feed: Feed) {
  emit('edit-feed', feed);
}

function handleDeleteFeed(id: number) {
  emit('delete-feed', id);
}

function handleBatchDelete(ids: number[]) {
  emit('batch-delete', ids);
}

function handleBatchMove(ids: number[]) {
  emit('batch-move', ids);
}

function handleBatchEnableImageMode(ids: number[]) {
  emit('batch-enable-image-mode', ids);
}

function handleBatchDisableImageMode(ids: number[]) {
  emit('batch-disable-image-mode', ids);
}

function handleBatchUpdateVideoMode(payload: { ids: number[]; enabled: boolean }) {
  emit('batch-update-video-mode', payload);
}


function handleSelectFeed(feedId: number) {
  emit('select-feed', feedId);
}
</script>

<template>
  <div class="space-y-4 sm:space-y-6">
    <DataManagementSettings
      @import-opml="handleImportOPML"
      @export-opml="handleExportOPML"
      @cleanup-database="handleCleanupDatabase"
    />

    <FeedManagementSettings
      :image-gallery-enabled="settings.image_gallery_enabled"
      @add-feed="handleAddFeed"
      @edit-feed="handleEditFeed"
      @delete-feed="handleDeleteFeed"
      @batch-delete="handleBatchDelete"
      @batch-move="handleBatchMove"
      @batch-enable-image-mode="handleBatchEnableImageMode"
      @batch-disable-image-mode="handleBatchDisableImageMode"
      @batch-update-video-mode="handleBatchUpdateVideoMode"
      @select-feed="handleSelectFeed"
    />

    <DiscoverySettings @discover-all="handleDiscoverAll" />
  </div>
</template>
