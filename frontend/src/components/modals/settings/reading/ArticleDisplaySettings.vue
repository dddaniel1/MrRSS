<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { PhArticle, PhImage, PhSquaresFour } from '@phosphor-icons/vue';
import { SettingGroup, SettingWithSelect, SettingWithToggle } from '@/components/settings';
import '@/components/settings/styles.css';
import type { SettingsData } from '@/types/settings';

const { t } = useI18n();

interface Props {
  settings: SettingsData;
}

const props = defineProps<Props>();

const emit = defineEmits<{
  'update:settings': [settings: SettingsData];
}>();

function updateSetting(key: keyof SettingsData, value: any) {
  emit('update:settings', {
    ...props.settings,
    [key]: value,
  });
}
</script>

<template>
  <SettingGroup :icon="PhArticle" :title="t('setting.tab.articleDisplay')">
    <SettingWithSelect
      :icon="PhArticle"
      :title="t('setting.reading.defaultViewMode')"
      :description="t('setting.reading.defaultViewModeDesc')"
      :model-value="settings.default_view_mode"
      :options="[
        { value: 'original', label: t('article.action.viewModeOriginal') },
        { value: 'rendered', label: t('article.action.viewModeRendered') },
        { value: 'external', label: t('article.action.viewModeExternal') },
      ]"
      width="md"
      @update:model-value="updateSetting('default_view_mode', $event)"
    />

    <SettingWithToggle
      :icon="PhImage"
      :title="t('setting.reading.showArticlePreviewImages')"
      :description="t('setting.reading.showArticlePreviewImagesDesc')"
      :model-value="settings.show_article_preview_images"
      @update:model-value="updateSetting('show_article_preview_images', $event)"
    />

    <SettingWithSelect
      :icon="PhSquaresFour"
      :title="t('setting.reading.articleListLayout')"
      :description="t('setting.reading.articleListLayoutDesc')"
      :model-value="settings.article_layout_mode || (settings.compact_mode ? 'compact' : 'normal')"
      :options="[
        { value: 'normal', label: t('setting.reading.articleListLayoutNormal') },
        { value: 'compact', label: t('setting.reading.articleListLayoutCompact') },
        { value: 'grid', label: t('setting.reading.articleListLayoutGrid') },
      ]"
      width="md"
      @update:model-value="updateSetting('article_layout_mode', $event)"
    />
  </SettingGroup>
</template>

<style scoped>
@reference "../../../../style.css";
</style>
