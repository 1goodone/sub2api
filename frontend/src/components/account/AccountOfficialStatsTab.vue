<template>
  <div class="space-y-4">
    <div class="flex flex-col gap-3 rounded-xl border border-gray-200 bg-gray-50 p-3 dark:border-dark-500 dark:bg-dark-700/50 lg:flex-row lg:items-center lg:justify-between">
      <div class="min-w-0 text-xs text-gray-500 dark:text-gray-400">
        <div class="flex items-center gap-2">
          <Icon name="server" size="sm" class="text-emerald-600 dark:text-emerald-400" />
          <span>{{ t('admin.accounts.officialUsage.source') }}: ChatGPT Wham</span>
        </div>
        <p class="mt-1 truncate">
          {{ t('admin.accounts.officialUsage.lastSync') }}: {{ data ? formatDateTime(data.fetched_at) : '-' }}
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <div class="inline-flex rounded-lg border border-gray-200 bg-white p-1 dark:border-dark-500 dark:bg-dark-700">
          <button
            v-for="option in ranges"
            :key="option.value"
            type="button"
            :class="[
              'rounded-md px-3 py-1.5 text-xs font-medium transition-colors',
              range === option.value
                ? 'bg-gray-900 text-white dark:bg-gray-100 dark:text-gray-900'
                : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-600'
            ]"
            @click="range = option.value"
          >
            {{ option.label }}
          </button>
        </div>
        <button
          type="button"
          class="inline-flex items-center gap-1.5 rounded-lg border border-gray-200 bg-white px-3 py-2 text-xs font-medium text-gray-700 transition-colors hover:bg-gray-50 disabled:opacity-50 dark:border-dark-500 dark:bg-dark-700 dark:text-gray-200 dark:hover:bg-dark-600"
          :disabled="loading"
          @click="load(true)"
        >
          <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
          {{ t('admin.accounts.officialUsage.refresh') }}
        </button>
      </div>
    </div>

    <div v-if="loading && !data" class="flex items-center justify-center py-16">
      <LoadingSpinner />
    </div>

    <div
      v-else-if="loadError"
      class="rounded-xl border border-red-200 bg-red-50 p-5 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-900/20 dark:text-red-300"
    >
      {{ loadError }}
    </div>

    <template v-else-if="data">
      <div
        v-if="data.state !== 'ok' && data.state !== 'stale'"
        :class="['rounded-xl border p-5', statePanelClass(data.state)]"
      >
        <div class="flex items-start gap-3">
          <Icon name="exclamationTriangle" size="md" :class="['mt-0.5', stateAccentClass(data.state)]" />
          <div>
            <p :class="['font-semibold', stateTitleClass(data.state)]">{{ stateLabel(data.state) }}</p>
            <p :class="['mt-1 text-sm', stateTextClass(data.state)]">{{ data.message }}</p>
          </div>
        </div>
      </div>

      <template v-else>
        <div
          v-if="data.stale || data.breakdown_error"
          class="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-300"
        >
          {{ data.refresh_error || data.breakdown_error }}
        </div>

        <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <div class="card border-emerald-200 p-4 dark:border-emerald-900/40">
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.officialUsage.officialCost') }}</p>
            <p class="mt-2 text-2xl font-bold text-emerald-600 dark:text-emerald-400">${{ formatCost(data.totals.usd) }}</p>
            <p class="mt-1 text-xs text-gray-400">{{ formatNumber(data.totals.credits) }} credits</p>
          </div>
          <div class="card p-4">
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.officialUsage.tokens') }}</p>
            <p class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">{{ formatCompact(data.totals.tokens) }}</p>
            <p class="mt-1 text-xs text-gray-400">{{ data.totals.settled_days }} {{ t('admin.accounts.officialUsage.settledDays') }}</p>
          </div>
          <div class="card p-4">
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.officialUsage.turns') }}</p>
            <p class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">{{ formatCompact(data.totals.turns) }}</p>
            <p class="mt-1 text-xs text-gray-400">{{ formatCompact(data.totals.threads) }} threads</p>
          </div>
          <div class="card border-blue-200 p-4 dark:border-blue-900/40">
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.officialUsage.cycleEstimate') }}</p>
            <p class="mt-2 text-2xl font-bold text-blue-600 dark:text-blue-400">
              {{ data.cycle.available ? `$${formatCost(data.cycle.estimated_usd || 0)}` : '-' }}
            </p>
            <p class="mt-1 text-xs text-gray-400">
              {{ data.cycle.available ? `${formatNumber(data.cycle.utilization || 0)}%` : cycleReason(data.cycle.reason) }}
            </p>
          </div>
        </div>

        <div v-if="data.cycle.available" class="card p-4">
          <div class="flex items-center justify-between gap-4 text-xs">
            <span class="font-medium text-gray-700 dark:text-gray-200">{{ t('admin.accounts.officialUsage.currentCycle') }}</span>
            <span class="text-gray-500 dark:text-gray-400">
              ${{ formatCost(data.cycle.used_usd || 0) }} · {{ formatNumber(data.cycle.utilization || 0) }}%
            </span>
          </div>
          <div class="mt-3 h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-600">
            <div class="h-full rounded-full bg-blue-500" :style="{ width: `${Math.min(100, data.cycle.utilization || 0)}%` }"></div>
          </div>
          <div class="mt-2 flex flex-wrap justify-between gap-2 text-xs text-gray-400">
            <span>{{ t('admin.accounts.officialUsage.estimateRange') }}: ${{ formatCost(data.cycle.estimated_low_usd || 0) }}–${{ formatCost(data.cycle.estimated_high_usd || 0) }}</span>
            <span>{{ t('admin.accounts.officialUsage.resetAt') }}: {{ formatDateTime(data.cycle.resets_at) }}</span>
          </div>
        </div>

        <div class="card p-4">
          <div class="mb-4 flex items-center justify-between">
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.accounts.officialUsage.dailyCost') }}</h3>
            <span class="text-xs text-gray-400">{{ t('admin.accounts.officialUsage.barHint') }}</span>
          </div>
          <div v-if="data.daily.length" class="max-h-80 space-y-3 overflow-y-auto pr-1">
            <div
              v-for="day in data.daily"
              :key="day.date"
              class="grid grid-cols-[64px_minmax(0,1fr)_64px] items-center gap-2 text-xs sm:grid-cols-[82px_minmax(0,1fr)_auto] sm:gap-3"
            >
              <div>
                <p class="font-medium text-gray-700 dark:text-gray-200">{{ formatDay(day.date) }}</p>
                <p :class="day.settled ? 'text-emerald-500' : 'text-amber-500'">{{ day.settled ? t('admin.accounts.officialUsage.settled') : t('admin.accounts.officialUsage.unsettled') }}</p>
              </div>
              <div class="h-8 overflow-hidden rounded-md bg-gray-100 dark:bg-dark-600">
                <div class="flex h-full min-w-[2px] items-center rounded-md bg-emerald-500/80 px-2 text-[11px] font-medium text-white" :style="{ width: dailyBarWidth(day.usd) }">
                  <span v-if="day.usd > 0">${{ formatCost(day.usd) }}</span>
                </div>
              </div>
              <div class="min-w-0 text-right text-gray-500 dark:text-gray-400 sm:min-w-[110px]">
                <p>{{ formatCompact(day.total_tokens) }} tok</p>
                <p>{{ day.turns }} turns</p>
              </div>
            </div>
          </div>
          <p v-else class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('admin.accounts.officialUsage.noData') }}</p>
        </div>

        <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
          <BreakdownList :title="t('admin.accounts.officialUsage.byClient')" :items="clientItems" />
          <BreakdownList :title="t('admin.accounts.officialUsage.byModel')" :items="modelItems" />
        </div>
      </template>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { Account, AccountOfficialUsageResponse, OfficialUsageState } from '@/types'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import BreakdownList from './AccountOfficialBreakdownList.vue'

const props = defineProps<{ account: Account }>()
const { t } = useI18n()
const range = ref<'7d' | '30d' | '90d' | 'all'>('30d')
const loading = ref(false)
const loadError = ref('')
const data = ref<AccountOfficialUsageResponse | null>(null)
let requestID = 0

const ranges = computed(() => [
  { value: '7d' as const, label: t('admin.accounts.officialUsage.range7d') },
  { value: '30d' as const, label: t('admin.accounts.officialUsage.range30d') },
  { value: '90d' as const, label: t('admin.accounts.officialUsage.range90d') },
  { value: 'all' as const, label: t('admin.accounts.officialUsage.rangeAll') }
])

const clientItems = computed(() => {
  const total = data.value?.totals.usd || 0
  return (data.value?.clients || []).map((item) => ({
    label: clientLabel(item.client_id),
    value: item.usd,
    share: total > 0 ? item.usd / total : 0,
    detail: `${formatCompact(item.tokens)} tok · ${item.turns} turns`
  }))
})

const modelItems = computed(() => {
  const total = data.value?.totals.usd || 0
  return (data.value?.models || []).map((item) => ({
    label: `${item.model} · ${item.speed}`,
    value: item.usd,
    share: total > 0 ? item.usd / total : 0,
    detail: `${formatNumber(item.credits)} credits`
  }))
})

watch([() => props.account.id, range], () => load(false), { immediate: true })

async function load(refresh: boolean) {
  const current = ++requestID
  loading.value = true
  loadError.value = ''
  try {
    const result = await adminAPI.accounts.getOfficialUsage(props.account.id, range.value, refresh)
    if (current === requestID) data.value = result
  } catch (error) {
    if (current === requestID) {
      loadError.value = error instanceof Error ? error.message : t('admin.accounts.officialUsage.loadFailed')
    }
  } finally {
    if (current === requestID) loading.value = false
  }
}

function stateLabel(state: OfficialUsageState) {
  return t(`admin.accounts.officialUsage.states.${state}`)
}

function isErrorState(state: OfficialUsageState) {
  return ['unauthorized', 'rate_limited', 'timeout', 'unavailable'].includes(state)
}

function statePanelClass(state: OfficialUsageState) {
  return isErrorState(state)
    ? 'border-red-200 bg-red-50 dark:border-red-900/50 dark:bg-red-900/20'
    : 'border-amber-200 bg-amber-50 dark:border-amber-900/50 dark:bg-amber-900/20'
}

function stateAccentClass(state: OfficialUsageState) {
  return isErrorState(state) ? 'text-red-600 dark:text-red-400' : 'text-amber-600 dark:text-amber-400'
}

function stateTitleClass(state: OfficialUsageState) {
  return isErrorState(state) ? 'text-red-900 dark:text-red-200' : 'text-amber-900 dark:text-amber-200'
}

function stateTextClass(state: OfficialUsageState) {
  return isErrorState(state) ? 'text-red-700 dark:text-red-300' : 'text-amber-700 dark:text-amber-300'
}

function cycleReason(reason?: string) {
  if (!reason) return '-'
  return t(`admin.accounts.officialUsage.cycleReasons.${reason}`)
}

function clientLabel(value: string) {
  const labels: Record<string, string> = {
    CODEX_CLI: 'Codex CLI',
    CODEX_DESKTOP_APP: 'Codex Desktop',
    CODEX_IDE_VSCODE: 'VS Code',
    CODEX_WORK_DESKTOP: 'Work Desktop',
    CODEX_SERVICE_EXEC: 'Service Exec',
    CODEX_SDK_TS: 'SDK',
    CODEX_WEB: 'Web',
    CODEX_UNKNOWN_DEFAULT: t('admin.accounts.officialUsage.gatewayTraffic')
  }
  return labels[value] || value.replace(/^CODEX_/, '').replace(/_/g, ' ')
}

function formatCost(value: number) {
  if (value >= 100) return value.toFixed(1)
  if (value >= 1) return value.toFixed(2)
  return value.toFixed(4)
}

function formatNumber(value: number) {
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(value)
}

function formatCompact(value: number) {
  return new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 2 }).format(value)
}

function formatDateTime(value?: string) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

function formatDay(value: string) {
  return new Date(`${value}T00:00:00`).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

function dailyBarWidth(value: number) {
  const max = Math.max(...(data.value?.daily.map((item) => item.usd) || [0]))
  return `${max > 0 ? Math.max(1, (value / max) * 100) : 0}%`
}
</script>
