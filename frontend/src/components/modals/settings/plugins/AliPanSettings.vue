<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  PhCloud,
  PhLink,
  PhLinkBreak,
  PhFolder,
  PhFileText,
  PhArrowClockwise,
  PhDownload,
} from '@phosphor-icons/vue';
import type { SettingsData } from '@/types/settings';
import { useAppStore } from '@/stores/app';
import { NestedSettingsContainer, SubSettingItem, InputControl } from '@/components/settings';

const { t } = useI18n();
const appStore = useAppStore();

interface Props {
  settings: SettingsData;
}

const props = defineProps<Props>();

const emit = defineEmits<{
  'update:settings': [settings: SettingsData];
}>();

const isBackingUp = ref(false);
const isRestoring = ref(false);
const isConnecting = ref(false);
const isDisconnecting = ref(false);
const isConnected = ref(false);
const hasBuiltinApp = ref(false);
let statusPollingTimer: number | null = null;

function updateSetting(key: keyof SettingsData, value: unknown) {
  emit('update:settings', {
    ...props.settings,
    [key]: value,
  });
}

const canRunActions = computed(() => props.settings.alipan_enabled && isConnected.value);

async function fetchOAuthStatus() {
  try {
    const resp = await fetch('/api/alipan/oauth/status');
    if (!resp.ok) return;

    const data = (await resp.json()) as { connected?: boolean; has_builtin?: boolean };
    isConnected.value = !!data.connected;
    hasBuiltinApp.value = !!data.has_builtin;
  } catch {
    // ignore polling errors
  }
}

function clearStatusPolling() {
  if (statusPollingTimer !== null) {
    window.clearInterval(statusPollingTimer);
    statusPollingTimer = null;
  }
}

function startStatusPolling() {
  clearStatusPolling();
  let checks = 0;
  statusPollingTimer = window.setInterval(async () => {
    checks += 1;
    await fetchOAuthStatus();
    if (isConnected.value || checks >= 60) {
      clearStatusPolling();
      isConnecting.value = false;
      if (isConnected.value) {
        window.showToast(t('setting.alipan.connectSuccess'), 'success');
      }
    }
  }, 2000);
}

async function openExternal(url: string) {
  try {
    const resp = await fetch('/api/browser/open', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ url }),
    });

    if (!resp.ok) {
      window.open(url, '_blank', 'noopener,noreferrer');
      return;
    }

    const data = (await resp.json()) as { redirect?: string };
    if (data.redirect) {
      window.open(data.redirect, '_blank', 'noopener,noreferrer');
    }
  } catch {
    window.open(url, '_blank', 'noopener,noreferrer');
  }
}

async function connectNow() {
  if (isConnecting.value) return;

  isConnecting.value = true;
  try {
    const resp = await fetch('/api/alipan/oauth/start', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
    });

    if (!resp.ok) {
      const msg = await resp.text();
      throw new Error(msg || t('setting.alipan.connectFailed'));
    }

    const data = (await resp.json()) as { authorize_url?: string };
    if (!data.authorize_url) {
      throw new Error(t('setting.alipan.connectFailed'));
    }

    await openExternal(data.authorize_url);
    window.showToast(t('setting.alipan.connectStarted'), 'info');
    startStatusPolling();
  } catch (error) {
    isConnecting.value = false;
    window.showToast(
      error instanceof Error ? error.message : t('setting.alipan.connectFailed'),
      'error'
    );
  }
}

async function refreshConnectionStatus() {
  await fetchOAuthStatus();
  if (isConnected.value) {
    window.showToast(t('setting.alipan.connectSuccess'), 'success');
  }
}

async function disconnectNow() {
  if (isDisconnecting.value) return;

  const confirmed = await window.showConfirm({
    title: t('setting.alipan.disconnect'),
    message: t('setting.alipan.disconnectConfirm'),
    isDanger: true,
  });
  if (!confirmed) return;

  isDisconnecting.value = true;
  try {
    const resp = await fetch('/api/alipan/oauth/disconnect', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
    });
    if (!resp.ok) {
      const msg = await resp.text();
      throw new Error(msg || t('setting.alipan.disconnectFailed'));
    }

    isConnected.value = false;
    window.showToast(t('setting.alipan.disconnectSuccess'), 'success');
  } catch (error) {
    window.showToast(
      error instanceof Error ? error.message : t('setting.alipan.disconnectFailed'),
      'error'
    );
  } finally {
    isDisconnecting.value = false;
  }
}

async function backupNow() {
  if (!canRunActions.value || isBackingUp.value) return;

  isBackingUp.value = true;
  try {
    const resp = await fetch('/api/alipan/backup', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
    });

    if (!resp.ok) {
      const msg = await resp.text();
      throw new Error(msg || t('setting.alipan.backupFailed'));
    }

    window.showToast(t('setting.alipan.backupSuccess'), 'success');
    updateSetting('alipan_last_backup_time', new Date().toISOString());
  } catch (error) {
    window.showToast(
      error instanceof Error ? error.message : t('setting.alipan.backupFailed'),
      'error'
    );
  } finally {
    isBackingUp.value = false;
  }
}

async function restoreNow() {
  if (!canRunActions.value || isRestoring.value) return;

  const confirmed = await window.showConfirm({
    title: t('setting.alipan.restoreNow'),
    message: t('setting.alipan.restoreConfirm'),
    isDanger: true,
  });
  if (!confirmed) return;

  isRestoring.value = true;
  try {
    const resp = await fetch('/api/alipan/restore', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        full_replace: true,
      }),
    });

    if (!resp.ok) {
      const msg = await resp.text();
      throw new Error(msg || t('setting.alipan.restoreFailed'));
    }

    await appStore.fetchFeeds();
    await appStore.fetchArticles();
    await appStore.fetchUnreadCounts();

    window.showToast(t('setting.alipan.restoreSuccess'), 'success');
    updateSetting('alipan_last_restore_time', new Date().toISOString());
  } catch (error) {
    window.showToast(
      error instanceof Error ? error.message : t('setting.alipan.restoreFailed'),
      'error'
    );
  } finally {
    isRestoring.value = false;
  }
}

function formatTime(timeStr: string): string {
  if (!timeStr) return t('common.time.never');
  const date = new Date(timeStr);
  if (Number.isNaN(date.getTime())) return t('common.time.never');
  return date.toLocaleString();
}

onMounted(() => {
  fetchOAuthStatus();
});

onUnmounted(() => {
  clearStatusPolling();
});
</script>

<template>
  <div class="setting-item">
    <div class="flex-1 flex items-center sm:items-start gap-2 sm:gap-3 min-w-0">
      <PhCloud :size="24" class="mt-0.5 shrink-0 text-accent" />
      <div class="flex-1 min-w-0">
        <div class="font-medium mb-0 sm:mb-1 text-sm sm:text-base">
          {{ t('setting.alipan.enabled') }}
        </div>
        <div class="text-xs text-text-secondary hidden sm:block">
          {{ t('setting.alipan.enabledDesc') }}
        </div>
      </div>
    </div>
    <input
      type="checkbox"
      :checked="props.settings.alipan_enabled"
      class="toggle"
      @change="updateSetting('alipan_enabled', ($event.target as HTMLInputElement).checked)"
    />
  </div>

  <NestedSettingsContainer v-if="props.settings.alipan_enabled">
    <SubSettingItem :icon="PhLink" :title="t('setting.alipan.account')">
      <template #description>
        <div>
          <div>
            {{ isConnected ? t('setting.alipan.connectedDesc') : t('setting.alipan.disconnectedDesc') }}
          </div>
          <div v-if="!hasBuiltinApp" class="text-xs text-text-secondary mt-1">
            {{ t('setting.alipan.builtinNotConfigured') }}
          </div>
        </div>
      </template>
      <div class="flex gap-2">
        <button
          v-if="!isConnected"
          class="btn-secondary"
          :disabled="isConnecting"
          @click="connectNow"
        >
          <PhLink :size="16" :class="{ 'animate-spin': isConnecting }" />
          {{ isConnecting ? t('setting.alipan.connecting') : t('setting.alipan.connect') }}
        </button>
        <button v-if="isConnected" class="btn-secondary danger" :disabled="isDisconnecting" @click="disconnectNow">
          <PhLinkBreak :size="16" :class="{ 'animate-spin': isDisconnecting }" />
          {{ isDisconnecting ? t('setting.alipan.disconnecting') : t('setting.alipan.disconnect') }}
        </button>
        <button class="btn-secondary" @click="refreshConnectionStatus">
          {{ t('setting.alipan.iHaveConnected') }}
        </button>
      </div>
    </SubSettingItem>

    <SubSettingItem
      :icon="PhFolder"
      :title="t('setting.alipan.backupFolder')"
      :description="t('setting.alipan.backupFolderDesc')"
    >
      <InputControl
        :model-value="props.settings.alipan_backup_folder"
        :placeholder="t('setting.alipan.backupFolderPlaceholder')"
        width="md"
        @update:model-value="updateSetting('alipan_backup_folder', $event)"
      />
    </SubSettingItem>

    <SubSettingItem
      :icon="PhFileText"
      :title="t('setting.alipan.backupFilename')"
      :description="t('setting.alipan.backupFilenameDesc')"
    >
      <InputControl
        :model-value="props.settings.alipan_backup_filename"
        :placeholder="t('setting.alipan.backupFilenamePlaceholder')"
        width="md"
        @update:model-value="updateSetting('alipan_backup_filename', $event)"
      />
    </SubSettingItem>

    <SubSettingItem :icon="PhArrowClockwise" :title="t('setting.alipan.backupNow')">
      <template #description>
        <div>
          <div>{{ t('setting.alipan.backupNowDesc') }}</div>
          <div class="text-xs text-text-secondary mt-1">
            {{ t('setting.alipan.lastBackup') }}:
            <span class="theme-number">{{ formatTime(props.settings.alipan_last_backup_time) }}</span>
          </div>
        </div>
      </template>
      <button class="btn-secondary" :disabled="!canRunActions || isBackingUp" @click="backupNow">
        <PhArrowClockwise :size="16" :class="{ 'animate-spin': isBackingUp }" />
        {{ isBackingUp ? t('setting.alipan.backingUp') : t('setting.alipan.backup') }}
      </button>
    </SubSettingItem>

    <SubSettingItem :icon="PhDownload" :title="t('setting.alipan.restoreNow')">
      <template #description>
        <div>
          <div>{{ t('setting.alipan.restoreNowDesc') }}</div>
          <div class="text-xs text-text-secondary mt-1">
            {{ t('setting.alipan.lastRestore') }}:
            <span class="theme-number">{{ formatTime(props.settings.alipan_last_restore_time) }}</span>
          </div>
        </div>
      </template>
      <button class="btn-secondary danger" :disabled="!canRunActions || isRestoring" @click="restoreNow">
        <PhDownload :size="16" :class="{ 'animate-spin': isRestoring }" />
        {{ isRestoring ? t('setting.alipan.restoring') : t('setting.alipan.restore') }}
      </button>
    </SubSettingItem>
  </NestedSettingsContainer>
</template>

<style scoped>
@reference "../../../../style.css";

.toggle {
  @apply w-10 h-5 appearance-none bg-bg-tertiary rounded-full relative cursor-pointer border border-border transition-colors checked:bg-accent checked:border-accent shrink-0;
}

.toggle::after {
  content: '';
  @apply absolute top-0.5 left-0.5 w-3.5 h-3.5 bg-white rounded-full shadow-sm transition-transform;
}

.toggle:checked::after {
  transform: translateX(20px);
}

.setting-item {
  @apply flex items-center sm:items-start justify-between gap-2 sm:gap-4 p-2 sm:p-3 rounded-lg bg-bg-secondary border border-border;
}

.btn-secondary {
  @apply bg-bg-tertiary border border-border text-text-primary px-3 sm:px-4 py-1.5 sm:py-2 rounded-md cursor-pointer flex items-center gap-1.5 sm:gap-2 font-medium hover:bg-bg-secondary transition-colors;
}

.btn-secondary:disabled {
  @apply cursor-not-allowed opacity-50;
}

.btn-secondary.danger {
  @apply border-red-500/40 text-red-500 hover:bg-red-500/10;
}

.theme-number {
  @apply text-accent font-semibold;
}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }

  to {
    transform: rotate(360deg);
  }
}

.animate-spin {
  animation: spin 1s linear infinite;
}
</style>
