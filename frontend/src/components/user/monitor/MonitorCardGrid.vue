<template>
  <div>
    <div
      v-if="loading && items.length === 0"
      class="overflow-hidden rounded-lg border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800"
    >
      <div
        v-for="i in 6"
        :key="i"
        class="h-[74px] border-b border-gray-100 px-5 py-4 last:border-b-0 dark:border-dark-700/70 animate-pulse"
      >
        <div class="flex items-start gap-3">
          <div class="w-9 h-9 rounded-xl bg-gray-200 dark:bg-dark-700"></div>
          <div class="flex-1 space-y-2">
            <div class="h-4 w-2/3 rounded bg-gray-200 dark:bg-dark-700"></div>
            <div class="h-3 w-1/2 rounded bg-gray-200 dark:bg-dark-700"></div>
          </div>
          <div class="h-6 w-16 rounded-full bg-gray-200 dark:bg-dark-700"></div>
        </div>
      </div>
    </div>

    <EmptyState
      v-else-if="items.length === 0"
      :title="t('channelStatus.empty.title')"
      :description="t('channelStatus.empty.description')"
    />

    <div
      v-else
      class="overflow-hidden rounded-lg border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800"
    >
      <div class="hidden h-11 grid-cols-[minmax(230px,1.35fr)_110px_120px_minmax(280px,2fr)_44px] items-center gap-4 border-b border-gray-200 bg-gray-50/70 px-5 text-xs font-semibold text-gray-500 dark:border-dark-700 dark:bg-dark-900/40 dark:text-gray-400 lg:grid">
        <span>{{ t('channelStatus.columns.name') }}</span>
        <span>{{ t('channelStatus.columns.status') }}</span>
        <span>{{ t('channelStatus.columns.availability') }}</span>
        <span>{{ t('channelStatus.columns.recentChecks') }}</span>
        <span class="sr-only">{{ t('channelStatus.actions.expand') }}</span>
      </div>
      <MonitorCard
        v-for="item in items"
        :key="item.id"
        :item="item"
        :window="window"
        :availability-value="resolveAvailability(item)"
        :countdown-seconds="countdownSeconds"
        :expanded="expandedId === item.id"
        @toggle="emit('toggle', item.id)"
        @open-detail="emit('openDetail', item)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { UserMonitorView, UserMonitorDetail } from '@/api/channelMonitor'
import EmptyState from '@/components/common/EmptyState.vue'
import MonitorCard from './MonitorCard.vue'

const props = defineProps<{
  items: UserMonitorView[]
  window: '7d' | '15d' | '30d'
  countdownSeconds: number
  loading: boolean
  detailCache: Record<number, UserMonitorDetail>
  expandedId: number | null
}>()

const emit = defineEmits<{
  (e: 'toggle', id: number): void
  (e: 'openDetail', item: UserMonitorView): void
}>()

const { t } = useI18n()

function resolveAvailability(item: UserMonitorView): number | null {
  if (props.window === '7d') {
    return item.availability_7d ?? null
  }
  const detail = props.detailCache[item.id]
  if (!detail) return null
  const primary = detail.models.find(m => m.model === item.primary_model)
  if (!primary) return null
  return props.window === '15d' ? primary.availability_15d ?? null : primary.availability_30d ?? null
}
</script>
