<template>
  <section class="py-3 md:py-4">
    <div class="mb-4">
      <h1 class="text-2xl font-semibold text-gray-950 dark:text-white">{{ t('channelStatus.title') }}</h1>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('channelStatus.description') }}</p>
    </div>
    <div class="flex flex-col gap-4 rounded-lg border border-gray-200 bg-white px-4 py-4 shadow-sm dark:border-dark-700 dark:bg-dark-800 sm:flex-row sm:items-center sm:justify-between sm:px-5">
      <div class="flex min-w-0 items-center gap-3">
        <span class="grid h-11 w-11 shrink-0 place-items-center rounded-full border" :class="summaryIconClass">
          <Icon :name="overallStatus === 'operational' ? 'checkCircle' : 'bolt'" size="lg" />
        </span>
        <div class="min-w-0">
          <p class="font-semibold text-gray-900 dark:text-gray-100">{{ summaryTitle }}</p>
          <p class="mt-0.5 text-sm text-gray-500 dark:text-gray-400">{{ summaryDetail }}</p>
        </div>
      </div>
      <div class="flex flex-wrap items-center gap-2 sm:justify-end">
      <div
        role="tablist"
        class="inline-flex p-0.5 rounded-xl bg-gray-100 dark:bg-dark-800 border border-gray-200/60 dark:border-dark-700/60 text-xs"
      >
        <button
          v-for="opt in windowOptions"
          :key="opt.value"
          type="button"
          role="tab"
          :aria-selected="window === opt.value"
          class="px-3 py-1 rounded-lg transition-colors"
          :class="window === opt.value
            ? 'bg-white dark:bg-dark-700 shadow-sm text-gray-900 dark:text-white font-semibold'
            : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'"
          @click="emit('update:window', opt.value)"
        >
          {{ opt.label }}
        </button>
      </div>

      <button
        type="button"
        class="h-8 w-8 rounded-lg flex items-center justify-center text-gray-500 hover:text-gray-700 hover:bg-gray-100 dark:text-gray-400 dark:hover:text-gray-200 dark:hover:bg-dark-700 transition-colors disabled:opacity-50"
        :disabled="loading"
        :title="t('common.refresh')"
        @click="emit('refresh')"
      >
        <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
      </button>

      <AutoRefreshButton
        v-if="autoRefresh"
        :enabled="autoRefresh.enabled.value"
        :interval-seconds="autoRefresh.intervalSeconds.value"
        :countdown="autoRefresh.countdown.value"
        :intervals="autoRefresh.intervals"
        @update:enabled="autoRefresh.setEnabled"
        @update:interval="autoRefresh.setInterval"
      />
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { UserMonitorView } from '@/api/channelMonitor'
import Icon from '@/components/icons/Icon.vue'
import AutoRefreshButton from '@/components/common/AutoRefreshButton.vue'
export type MonitorWindow = '7d' | '15d' | '30d'
export type OverallStatus = 'operational' | 'degraded'

const props = defineProps<{
  overallStatus: OverallStatus
  items: UserMonitorView[]
  intervalSeconds: number
  window: MonitorWindow
  loading: boolean
  autoRefresh?: {
    enabled: { value: boolean }
    intervalSeconds: { value: number }
    countdown: { value: number }
    intervals: readonly number[]
    setEnabled: (v: boolean) => void
    setInterval: (v: number) => void
  }
}>()

const emit = defineEmits<{
  (e: 'update:window', value: MonitorWindow): void
  (e: 'refresh'): void
}>()

const { t } = useI18n()

const windowOptions = computed<{ value: MonitorWindow; label: string }[]>(() => [
  { value: '7d', label: t('channelStatus.windowTab.7d') },
  { value: '15d', label: t('channelStatus.windowTab.15d') },
  { value: '30d', label: t('channelStatus.windowTab.30d') },
])

const counts = computed(() => ({
  operational: props.items.filter(item => item.primary_status === 'operational').length,
  degraded: props.items.filter(item => item.primary_status === 'degraded').length,
  failed: props.items.filter(item => item.primary_status === 'failed' || item.primary_status === 'error').length,
}))

const summaryTitle = computed(() => t(`channelStatus.summary.${props.overallStatus}`))
const summaryDetail = computed(() => t('channelStatus.summary.counts', counts.value))
const summaryIconClass = computed(() => props.overallStatus === 'operational'
  ? 'border-emerald-200 bg-emerald-50 text-emerald-600 dark:border-emerald-500/30 dark:bg-emerald-500/10 dark:text-emerald-300'
  : 'border-amber-200 bg-amber-50 text-amber-600 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300')

</script>
