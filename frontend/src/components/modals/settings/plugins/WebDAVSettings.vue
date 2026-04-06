<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  PhCloud,
  PhLink,
  PhUser,
  PhKey,
  PhFolderSimple,
  PhArrowsClockwise,
} from '@phosphor-icons/vue';
import type { SettingsData } from '@/types/settings';
import { NestedSettingsContainer, SubSettingItem, InputControl } from '@/components/settings';
import { buildAutoSavePayload } from '@/composables/core/useSettings.generated';

const { t } = useI18n();

interface Props {
  settings: SettingsData;
}

const props = defineProps<Props>();

const emit = defineEmits<{
  'update:settings': [settings: SettingsData];
}>();

const isSyncing = ref(false);

const urlError = computed(() => {
  if (!props.settings.webdav_enabled) return '';

  const value = props.settings.webdav_url.trim();
  if (!value) return t('common.form.requiredField');

  try {
    const parsed = new URL(value);
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
      return t('setting.webdav.invalidServerUrl');
    }
  } catch {
    return t('setting.webdav.invalidServerUrl');
  }

  return '';
});

const usernameError = computed(() => {
  if (!props.settings.webdav_enabled || props.settings.webdav_username.trim()) return '';
  return t('common.form.requiredField');
});

const passwordError = computed(() => {
  if (!props.settings.webdav_enabled || props.settings.webdav_password.trim()) return '';
  return t('common.form.requiredField');
});

const isConfigValid = computed(() => {
  return !urlError.value && !usernameError.value && !passwordError.value;
});

function updateSetting<K extends keyof SettingsData>(key: K, value: SettingsData[K]) {
  emit('update:settings', {
    ...props.settings,
    [key]: value,
  });
}

async function syncNow() {
  if (!isConfigValid.value) {
    window.showToast(t('setting.webdav.validationFailed'), 'error');
    return;
  }

  isSyncing.value = true;

  try {
    const settingsPayload = buildAutoSavePayload({ value: props.settings });

    const settingsResponse = await fetch('/api/settings', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(settingsPayload),
    });

    if (!settingsResponse.ok) {
      const settingsError = await settingsResponse.text();
      throw new Error(settingsError || t('common.errors.savingSettings'));
    }

    const response = await fetch('/api/webdav/sync', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
    });

    const data = await response.json().catch(() => null);
    if (!response.ok) {
      throw new Error(data?.error || data?.message || t('setting.webdav.syncFailed'));
    }

    updateSetting('webdav_last_sync_time', data?.last_sync_time ?? new Date().toISOString());
    window.showToast(
      t('setting.webdav.syncSuccess', {
        imported: data?.imported_count ?? 0,
        exported: data?.exported_count ?? 0,
      }),
      'success'
    );
  } catch (error) {
    window.showToast(
      error instanceof Error ? error.message : t('setting.webdav.syncFailed'),
      'error'
    );
  } finally {
    isSyncing.value = false;
  }
}

function formatSyncTime(timeStr: string): string {
  if (!timeStr) return t('setting.webdav.never');
  const date = new Date(timeStr);
  if (Number.isNaN(date.getTime())) return t('setting.webdav.never');

  const diffMs = Date.now() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);

  if (diffMins < 1) return t('setting.webdav.justNow');
  if (diffMins < 60) return t('setting.webdav.minsAgo', { count: diffMins });
  const diffHours = Math.floor(diffMins / 60);
  if (diffHours < 24) return t('setting.webdav.hoursAgo', { count: diffHours });
  const diffDays = Math.floor(diffHours / 24);
  return t('setting.webdav.daysAgo', { count: diffDays });
}
</script>

<template>
  <div class="setting-item">
    <div class="flex-1 flex items-center sm:items-start gap-2 sm:gap-3 min-w-0">
      <PhCloud :size="24" class="w-5 h-5 sm:w-6 sm:h-6 mt-0.5 shrink-0 text-accent" />
      <div class="flex-1 min-w-0">
        <div class="font-medium mb-0 sm:mb-1 text-sm sm:text-base">
          {{ t('setting.webdav.enabled') }}
        </div>
        <div class="text-xs text-text-secondary hidden sm:block">
          {{ t('setting.webdav.enabledDesc') }}
        </div>
      </div>
    </div>
    <input
      type="checkbox"
      :checked="props.settings.webdav_enabled"
      class="toggle"
      @change="updateSetting('webdav_enabled', ($event.target as HTMLInputElement).checked)"
    />
  </div>

  <NestedSettingsContainer v-if="props.settings.webdav_enabled">
    <SubSettingItem
      :icon="PhLink"
      :title="t('setting.webdav.serverUrl')"
      :description="t('setting.webdav.serverUrlDesc')"
      required
    >
      <InputControl
        type="url"
        :model-value="props.settings.webdav_url"
        :placeholder="t('setting.webdav.serverUrlPlaceholder')"
        :error="urlError"
        width="md"
        @update:model-value="updateSetting('webdav_url', $event)"
      />
    </SubSettingItem>

    <SubSettingItem
      :icon="PhUser"
      :title="t('setting.webdav.username')"
      :description="t('setting.webdav.usernameDesc')"
      required
    >
      <InputControl
        :model-value="props.settings.webdav_username"
        :placeholder="t('setting.webdav.usernamePlaceholder')"
        :error="usernameError"
        width="md"
        @update:model-value="updateSetting('webdav_username', $event)"
      />
    </SubSettingItem>

    <SubSettingItem
      :icon="PhKey"
      :title="t('setting.webdav.password')"
      :description="t('setting.webdav.passwordDesc')"
      required
    >
      <InputControl
        type="password"
        :model-value="props.settings.webdav_password"
        :placeholder="t('setting.webdav.passwordPlaceholder')"
        :error="passwordError"
        width="md"
        @update:model-value="updateSetting('webdav_password', $event)"
      />
    </SubSettingItem>

    <SubSettingItem
      :icon="PhFolderSimple"
      :title="t('setting.webdav.remotePath')"
      :description="t('setting.webdav.remotePathDesc')"
    >
      <InputControl
        :model-value="props.settings.webdav_remote_path"
        :placeholder="t('setting.webdav.remotePathPlaceholder')"
        width="md"
        @update:model-value="updateSetting('webdav_remote_path', $event)"
      />
    </SubSettingItem>

    <SubSettingItem
      :icon="PhArrowsClockwise"
      :title="t('setting.webdav.syncNow')"
      :description="t('setting.webdav.syncNowDesc')"
    >
      <template #description>
        <div>
          {{ t('setting.webdav.syncNowDesc') }}
          <div class="text-xs text-text-secondary mt-1">
            {{ t('setting.webdav.lastSync') }}:
            <span class="theme-number">{{ formatSyncTime(props.settings.webdav_last_sync_time) }}</span>
          </div>
        </div>
      </template>
      <button class="btn-secondary" :disabled="isSyncing || !isConfigValid" @click="syncNow">
        <PhArrowsClockwise :size="16" class="sm:w-5 sm:h-5" :class="{ 'animate-spin': isSyncing }" />
        {{ isSyncing ? t('setting.webdav.syncing') : t('setting.webdav.sync') }}
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

.theme-number {
  @apply text-accent font-semibold;
}
</style>
