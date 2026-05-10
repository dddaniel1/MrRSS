<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  PhFolderOpen,
  PhHash,
  PhTag,
  PhTextT,
  PhTimer,
  PhImages,
  PhTestTube,
  PhArrowsClockwise,
} from '@phosphor-icons/vue';
import type { SettingsData } from '@/types/settings';
import {
  NestedSettingsContainer,
  SubSettingItem,
  InputControl,
  NumberControl,
  ToggleControl,
  InfoBox,
} from '@/components/settings';

const { t } = useI18n();

interface Props {
  settings: SettingsData;
}

interface EagleFolderOption {
  id: string;
  name: string;
  path: string;
  depth: number;
}

const props = defineProps<Props>();

const emit = defineEmits<{
  'update:settings': [settings: SettingsData];
}>();

const folders = ref<EagleFolderOption[]>([]);
const isLoadingFolders = ref(false);
const isTesting = ref(false);

const folderOptions = computed(() => [
  { id: '', path: t('setting.plugins.eagle.noFolder') },
  ...(props.settings.eagle_folder_id &&
  !folders.value.some((folder) => folder.id === props.settings.eagle_folder_id)
    ? [
        {
          id: props.settings.eagle_folder_id,
          path: props.settings.eagle_folder_name || props.settings.eagle_folder_id,
        },
      ]
    : []),
  ...folders.value,
]);

function updateSetting(key: keyof SettingsData, value: any) {
  emit('update:settings', {
    ...props.settings,
    [key]: value,
  });
}

function handleFolderChange(event: Event) {
  const folderId = (event.target as HTMLSelectElement).value;
  const folder = folders.value.find((item) => item.id === folderId);
  emit('update:settings', {
    ...props.settings,
    eagle_folder_id: folderId,
    eagle_folder_name: folder?.path || '',
  });
}

async function refreshFolders() {
  isLoadingFolders.value = true;
  try {
    const response = await fetch('/api/eagle/folders');
    if (!response.ok) {
      throw new Error(await response.text());
    }

    const data = await response.json();
    folders.value = Array.isArray(data.folders) ? data.folders : [];
    window.showToast(t('setting.plugins.eagle.foldersLoaded'), 'success');
  } catch (error) {
    window.showToast(
      error instanceof Error ? error.message : t('setting.plugins.eagle.foldersLoadFailed'),
      'error'
    );
  } finally {
    isLoadingFolders.value = false;
  }
}

async function testConnection() {
  isTesting.value = true;
  try {
    const response = await fetch('/api/eagle/test', { method: 'POST' });
    const data = await response.json().catch(() => ({}));
    if (!response.ok || !data.success) {
      throw new Error(data.error || t('setting.plugins.eagle.connectionFailed'));
    }

    window.showToast(t('setting.plugins.eagle.connectionSuccessful'), 'success');
  } catch (error) {
    window.showToast(
      error instanceof Error ? error.message : t('setting.plugins.eagle.connectionFailed'),
      'error'
    );
  } finally {
    isTesting.value = false;
  }
}
</script>

<template>
  <div class="setting-item">
    <div class="flex-1 flex items-center sm:items-start gap-2 sm:gap-3 min-w-0">
      <img
        src="/assets/plugin_icons/eagle.png"
        alt="Eagle"
        class="w-5 h-5 sm:w-6 sm:h-6 mt-0.5 shrink-0"
      />
      <div class="flex-1 min-w-0">
        <div class="font-medium mb-0 sm:mb-1 text-sm sm:text-base">
          {{ t('setting.plugins.eagle.integration') }}
        </div>
        <div class="text-xs text-text-secondary hidden sm:block">
          {{ t('setting.plugins.eagle.integrationDescription') }}
        </div>
      </div>
    </div>
    <input
      type="checkbox"
      :checked="props.settings.eagle_enabled"
      class="toggle"
      @change="updateSetting('eagle_enabled', ($event.target as HTMLInputElement).checked)"
    />
  </div>

  <NestedSettingsContainer v-if="props.settings.eagle_enabled">
    <InfoBox :content="t('setting.plugins.eagle.info')" />

    <SubSettingItem
      :icon="PhTestTube"
      :title="t('setting.plugins.eagle.apiUrl')"
      :description="t('setting.plugins.eagle.apiUrlDesc')"
      required
    >
      <InputControl
        :model-value="props.settings.eagle_api_url"
        placeholder="http://localhost:41595"
        width="lg"
        @update:model-value="updateSetting('eagle_api_url', $event)"
      />
    </SubSettingItem>

    <SubSettingItem
      :icon="PhFolderOpen"
      :title="t('setting.plugins.eagle.folder')"
      :description="t('setting.plugins.eagle.folderDesc')"
    >
      <div class="flex flex-col sm:flex-row gap-2 items-stretch sm:items-center w-full sm:w-auto">
        <select
          class="input-field w-full sm:w-72"
          :value="props.settings.eagle_folder_id"
          @change="handleFolderChange"
        >
          <option v-for="folder in folderOptions" :key="folder.id" :value="folder.id">
            {{ folder.path }}
          </option>
        </select>
        <button class="btn-secondary" :disabled="isLoadingFolders" @click="refreshFolders">
          <PhArrowsClockwise :size="16" :class="isLoadingFolders ? 'animate-spin' : ''" />
          {{ isLoadingFolders ? t('common.checking') : t('setting.plugins.eagle.refreshFolders') }}
        </button>
      </div>
    </SubSettingItem>

    <SubSettingItem
      :icon="PhTag"
      :title="t('setting.plugins.eagle.tags')"
      :description="t('setting.plugins.eagle.tagsDesc')"
    >
      <InputControl
        :model-value="props.settings.eagle_tags"
        placeholder="MrRSS, RSS"
        width="lg"
        @update:model-value="updateSetting('eagle_tags', $event)"
      />
    </SubSettingItem>

    <SubSettingItem
      :icon="PhHash"
      :title="t('setting.plugins.eagle.includeFeedTag')"
      :description="t('setting.plugins.eagle.includeFeedTagDesc')"
    >
      <ToggleControl
        :model-value="props.settings.eagle_include_feed_tag"
        @update:model-value="updateSetting('eagle_include_feed_tag', $event)"
      />
    </SubSettingItem>

    <SubSettingItem
      :icon="PhTextT"
      :title="t('setting.plugins.eagle.nameTemplate')"
      :description="t('setting.plugins.eagle.nameTemplateDesc')"
    >
      <InputControl
        :model-value="props.settings.eagle_name_template"
        placeholder="{title} {timestamp} {index}"
        width="lg"
        @update:model-value="updateSetting('eagle_name_template', $event)"
      />
    </SubSettingItem>

    <SubSettingItem
      :icon="PhTimer"
      :title="t('setting.plugins.eagle.timeout')"
      :description="t('setting.plugins.eagle.timeoutDesc')"
    >
      <NumberControl
        :model-value="props.settings.eagle_timeout_seconds"
        :min="5"
        :max="300"
        suffix="s"
        width="md"
        @update:model-value="updateSetting('eagle_timeout_seconds', $event)"
      />
    </SubSettingItem>

    <SubSettingItem
      :icon="PhImages"
      :title="t('setting.plugins.eagle.maxBatchSize')"
      :description="t('setting.plugins.eagle.maxBatchSizeDesc')"
    >
      <NumberControl
        :model-value="props.settings.eagle_max_batch_size"
        :min="1"
        :max="500"
        width="md"
        @update:model-value="updateSetting('eagle_max_batch_size', $event)"
      />
    </SubSettingItem>

    <SubSettingItem
      :icon="PhTestTube"
      :title="t('setting.plugins.eagle.testConnection')"
      :description="t('setting.plugins.eagle.testConnectionDesc')"
    >
      <button class="btn-secondary" :disabled="isTesting" @click="testConnection">
        {{
          isTesting ? t('setting.plugins.eagle.testing') : t('setting.plugins.eagle.testConnection')
        }}
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

.input-field {
  @apply p-1.5 sm:p-2.5 border border-border rounded-md bg-bg-secondary text-text-primary focus:border-accent focus:outline-none transition-colors text-xs sm:text-sm;
}

.btn-secondary {
  @apply bg-bg-tertiary border border-border text-text-primary px-3 sm:px-4 py-1.5 sm:py-2 rounded-md cursor-pointer flex items-center justify-center gap-1.5 sm:gap-2 font-medium hover:bg-bg-secondary transition-colors text-xs sm:text-sm whitespace-nowrap;
}
.btn-secondary:disabled {
  @apply cursor-not-allowed opacity-50;
}
</style>
