<script setup lang="ts">
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { PhHardDrives, PhUpload, PhDownload, PhBroom, PhToggleLeft, PhToggleRight } from '@phosphor-icons/vue';
import { ButtonControl } from '@/components/settings';
import { SettingGroup } from '@/components/settings';

const { t } = useI18n();

const useRSSHubProtocol = ref(false);

const emit = defineEmits<{
  'import-opml': [];
  'export-opml': [useRSSHubProtocol: boolean];
  'cleanup-database': [];
}>();

function handleImportOPML() {
  emit('import-opml');
}

function handleExportOPML() {
  emit('export-opml', useRSSHubProtocol.value);
}

function handleCleanupDatabase() {
  emit('cleanup-database');
}

function toggleRSSHubProtocol() {
  useRSSHubProtocol.value = !useRSSHubProtocol.value;
}
</script>

<template>
  <SettingGroup :icon="PhHardDrives" :title="t('setting.database.dataManagement')">
    <div class="flex flex-col sm:flex-row gap-2 sm:gap-3">
      <ButtonControl
        :label="t('modal.opml.import')"
        :icon="PhDownload"
        type="secondary"
        class="flex-1 justify-center text-sm sm:text-base"
        @click="handleImportOPML"
      />
      <ButtonControl
        :label="t('modal.opml.export')"
        :icon="PhUpload"
        type="secondary"
        class="flex-1 justify-center text-sm sm:text-base"
        @click="handleExportOPML"
      />
    </div>

    <!-- RSSHub Protocol Toggle -->
    <div
      class="flex items-center justify-between p-3 rounded-lg border border-border cursor-pointer hover:bg-bg-secondary transition-colors"
      @click="toggleRSSHubProtocol"
    >
      <div class="flex items-center gap-3">
        <component
          :is="useRSSHubProtocol ? PhToggleRight : PhToggleLeft"
          class="w-6 h-6"
          :class="useRSSHubProtocol ? 'text-accent' : 'text-text-secondary'"
        />
        <div>
          <div class="text-sm font-medium text-text-primary">{{ t('modal.opml.rsshubProtocol') }}</div>
          <div class="text-xs text-text-secondary">{{ t('modal.opml.rsshubProtocolDesc') }}</div>
        </div>
      </div>
    </div>

    <ButtonControl
      :label="t('setting.database.cleanDatabase')"
      :icon="PhBroom"
      type="danger"
      class="w-full justify-center text-sm sm:text-base"
      @click="handleCleanupDatabase"
    />
  </SettingGroup>
</template>

<style scoped>
@reference "../../../../style.css";
</style>
