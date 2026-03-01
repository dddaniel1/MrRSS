<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import {
  PhTrash,
  PhCheckCircle,
  PhEye,
  PhEyeSlash,
  PhFunnel,
} from '@phosphor-icons/vue';

interface Props {
  currentFilter: string;
  showOnlyUnread: boolean;
  activeFiltersCount: number;
  layoutIcon: any;
  layoutTitle: string;
}

defineProps<Props>();

defineEmits<{
  cycleLayout: [];
  clearReadLater: [];
  markAllRead: [];
  toggleShowOnlyUnread: [];
  openFilter: [];
}>();

const { t } = useI18n();
</script>

<template>
  <div class="flex items-center gap-1 sm:gap-2">
    <button
      class="header-action-btn"
      :title="`${t('setting.reading.articleListLayout')}: ${layoutTitle}`"
      @click="$emit('cycleLayout')"
    >
      <component :is="layoutIcon" :size="16" />
    </button>

    <button
      v-if="currentFilter === 'readLater'"
      class="header-action-btn hover:text-red-500"
      :title="t('common.clearReadLater')"
      @click="$emit('clearReadLater')"
    >
      <PhTrash :size="18" class="sm:w-5 sm:h-5" />
    </button>

    <button class="header-action-btn" :title="t('article.action.markAllRead')" @click="$emit('markAllRead')">
      <PhCheckCircle :size="18" class="sm:w-5 sm:h-5" />
    </button>

    <button
      class="header-action-btn"
      :class="showOnlyUnread ? 'text-accent' : ''"
      :title="t('setting.reading.showOnlyUnread')"
      @click="$emit('toggleShowOnlyUnread')"
    >
      <component :is="showOnlyUnread ? PhEye : PhEyeSlash" :size="18" class="sm:w-5 sm:h-5" />
    </button>

    <div class="relative">
      <button
        class="header-action-btn"
        :class="activeFiltersCount > 0 ? 'filter-active' : ''"
        :title="t('modal.filter.filter')"
        @click="$emit('openFilter')"
      >
        <PhFunnel :size="18" class="sm:w-5 sm:h-5" />
      </button>
      <div
        v-if="activeFiltersCount > 0"
        class="absolute -top-1 -right-1 bg-accent text-white text-[9px] sm:text-[10px] font-bold rounded-full min-w-[14px] sm:min-w-[16px] h-3.5 sm:h-4 px-0.5 sm:px-1 flex items-center justify-center"
      >
        {{ activeFiltersCount }}
      </div>
    </div>
  </div>
</template>

<style scoped>
@reference "../../style.css";

.header-action-btn {
  @apply text-text-secondary hover:text-text-primary hover:bg-bg-tertiary p-1 sm:p-1.5 rounded transition-colors flex items-center justify-center;
}

.filter-active {
  @apply text-accent border-accent;
  background-color: rgba(59, 130, 246, 0.1);
}
</style>
