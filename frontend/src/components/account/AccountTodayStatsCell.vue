<template>
  <div>
    <div v-if="props.loading && !props.stats" class="space-y-1">
      <div class="h-4 w-28 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
      <div class="h-3 w-64 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
      <div class="h-3 w-64 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
    </div>

    <div v-else-if="props.error && !props.stats" class="text-xs text-red-500">
      {{ props.error }}
    </div>

    <div v-else-if="props.stats" class="min-w-[17rem] space-y-1.5 text-xs">
      <div class="flex items-center justify-between gap-3">
        <div
          class="flex h-5 items-end gap-0.5"
          data-test="recent-call-bars"
          :aria-label="t('admin.accounts.stats.recentActivity')"
        >
          <template v-if="props.stats.recent_calls?.length">
            <span
              v-for="(call, index) in props.stats.recent_calls"
              :key="`${call.created_at}-${index}`"
              class="h-4 w-1.5 rounded-[1px]"
              :class="call.success ? 'bg-emerald-500' : 'bg-red-500'"
              :data-test="call.success ? 'recent-call-success' : 'recent-call-failure'"
              :title="call.success ? t('common.success') : t('common.error')"
            ></span>
          </template>
          <span v-else class="text-gray-400">-</span>
        </div>
        <button
          type="button"
          class="shrink-0 font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
          data-test="view-stats"
          @click="emit('view-stats')"
        >
          {{ t('admin.accounts.viewStats') }}
        </button>
      </div>

      <div class="grid grid-cols-2 gap-x-4 gap-y-1">
        <div class="flex items-baseline justify-between gap-2">
          <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.stats.todayRequests') }}</span>
          <span class="font-medium text-emerald-600 dark:text-emerald-400" data-test="today-requests">
            {{ formatNumber(props.stats.requests) }}
          </span>
        </div>
        <div class="flex items-baseline justify-between gap-2">
          <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.stats.lifetimeRequests') }}</span>
          <span class="font-medium text-gray-900 dark:text-gray-100" data-test="lifetime-requests">
            {{ formatNumber(props.stats.lifetime_requests ?? 0) }}
          </span>
        </div>
        <div class="flex items-baseline justify-between gap-2">
          <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.stats.todayTokens') }}</span>
          <span class="font-medium text-emerald-600 dark:text-emerald-400" data-test="today-tokens">
            {{ formatTokens(props.stats.tokens) }}
          </span>
        </div>
        <div class="flex items-baseline justify-between gap-2">
          <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.stats.lifetimeTokens') }}</span>
          <span class="font-medium text-gray-900 dark:text-gray-100" data-test="lifetime-tokens">
            {{ formatTokens(props.stats.lifetime_tokens ?? 0) }}
          </span>
        </div>
        <div class="flex items-baseline justify-between gap-2">
          <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.stats.todayCost') }}</span>
          <span class="font-medium text-emerald-600 dark:text-emerald-400" data-test="today-cost">
            {{ formatCurrency(props.stats.cost) }}
          </span>
        </div>
        <div class="flex items-baseline justify-between gap-2">
          <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.stats.lifetimeCost') }}</span>
          <span class="font-medium text-gray-900 dark:text-gray-100" data-test="lifetime-cost">
            {{ formatCurrency(props.stats.lifetime_cost ?? 0) }}
          </span>
        </div>
      </div>
    </div>

    <div v-else class="text-xs text-gray-400">-</div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { WindowStats } from '@/types'
import { formatCurrency, formatNumber } from '@/utils/format'

const props = withDefaults(
  defineProps<{
    stats?: WindowStats | null
    loading?: boolean
    error?: string | null
  }>(),
  {
    stats: null,
    loading: false,
    error: null
  }
)

const emit = defineEmits<{
  (event: 'view-stats'): void
}>()

const { t } = useI18n()

const formatTokens = (tokens: number): string => {
  if (tokens >= 1000000) return `${(tokens / 1000000).toFixed(2)}M`
  if (tokens >= 1000) return `${(tokens / 1000).toFixed(1)}K`
  return tokens.toString()
}
</script>
