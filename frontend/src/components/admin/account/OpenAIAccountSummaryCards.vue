<template>
  <div class="openai-summary-grid mt-3" data-test="openai-account-summary">
    <section
      v-for="card in cards"
      :key="card.key"
      class="min-w-0 rounded-xl border bg-white p-3 shadow-sm transition-colors dark:bg-dark-800"
      :class="card.active
        ? 'border-primary-400 ring-1 ring-primary-200 dark:border-primary-600 dark:ring-primary-900'
        : 'border-gray-200 dark:border-dark-700'"
    >
      <div class="flex items-start justify-between gap-2">
        <div class="truncate text-xs font-semibold text-gray-600 dark:text-gray-300">
          {{ card.title }}
        </div>
        <span
          class="inline-flex shrink-0 items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold"
          :class="card.badgeClass"
        >
          <span class="h-1.5 w-1.5 rounded-full" :class="card.dotClass" />
          {{ card.badge }}
        </span>
      </div>

      <div class="mt-2 grid gap-1.5" :class="card.items.length > 1 ? 'grid-cols-2' : 'grid-cols-1'">
        <button
          v-for="item in card.items"
          :key="item.key"
          type="button"
          class="min-w-0 rounded-lg px-2 py-1.5 text-left transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500"
          :class="item.active
            ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-200'
            : 'text-gray-800 hover:bg-gray-50 dark:text-gray-100 dark:hover:bg-dark-700'"
          :aria-label="item.ariaLabel"
          :aria-pressed="item.active"
          :data-test="`openai-summary-${item.key}`"
          @click="$emit('select', item.filter)"
        >
          <div v-if="item.label" class="truncate text-[11px] font-medium text-gray-500 dark:text-gray-400">
            {{ item.label }}
          </div>
          <div
            class="mt-0.5 truncate font-semibold tabular-nums tracking-tight"
            :class="item.compact ? 'text-base' : 'text-2xl'"
            :title="item.value"
          >
            {{ item.value }}
          </div>
          <div v-if="item.hint" class="mt-0.5 truncate text-[10px] text-gray-400 dark:text-gray-500">
            {{ item.hint }}
          </div>
        </button>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { OpenAIAccountSummary } from '@/api/admin/accounts'

type OpenAIAccountSummaryFilter = {
  type: 'oauth' | 'apikey'
  status?: 'active' | 'rate_limited' | 'error'
  quota_window?: '5h' | '7d'
}

type SummaryItem = {
  key: string
  label: string
  value: string
  hint?: string
  compact?: boolean
  ariaLabel: string
  active: boolean
  filter: OpenAIAccountSummaryFilter
}

type SummaryCard = {
  key: string
  title: string
  badge: string
  badgeClass: string
  dotClass: string
  active: boolean
  items: SummaryItem[]
}

const props = defineProps<{
  summary: OpenAIAccountSummary | null
  filters: Record<string, unknown>
}>()

defineEmits<{
  select: [filter: OpenAIAccountSummaryFilter]
}>()

const { t } = useI18n()
const value = (count: number | undefined) => props.summary ? String(count ?? 0) : '—'
const concurrency = (item: { used: number; total: number } | undefined) =>
  props.summary ? `${item?.used ?? 0}/${item?.total ?? 0}` : '—'
const money = (amount: number) => `$${amount.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
const quota = (item: { remaining_cost: number | null; estimated_cost: number | null } | undefined) => {
  if (!props.summary || item?.remaining_cost == null || item.estimated_cost == null) return '—'
  return `${money(item.remaining_cost)} / ${money(item.estimated_cost)}`
}
const typeActive = (type: string) => props.filters.type === type && !props.filters.status && !props.filters.quota_window
const statusActive = (status: string) => props.filters.type === 'oauth' && props.filters.status === status && !props.filters.quota_window
const quotaActive = (window: string) => props.filters.type === 'oauth' && props.filters.quota_window === window

const cards = computed<SummaryCard[]>(() => [
  {
    key: 'accounts',
    title: t('admin.accounts.openaiSummary.totalAccounts'),
    badge: t('admin.accounts.openaiSummary.all'),
    badgeClass: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300',
    dotClass: 'bg-gray-500',
    active: typeActive('oauth') || typeActive('apikey'),
    items: [
      { key: 'accounts-oauth', label: 'OAuth', value: value(props.summary?.accounts.oauth), ariaLabel: 'OAuth', active: typeActive('oauth'), filter: { type: 'oauth' as const } },
      { key: 'accounts-apikey', label: 'APIKey', value: value(props.summary?.accounts.apikey), ariaLabel: 'APIKey', active: typeActive('apikey'), filter: { type: 'apikey' as const } }
    ]
  },
  {
    key: 'normal',
    title: t('admin.accounts.openaiSummary.normalAccounts'),
    badge: t('admin.accounts.openaiSummary.normal'),
    badgeClass: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300',
    dotClass: 'bg-emerald-500',
    active: statusActive('active'),
    items: [
      { key: 'status-normal', label: 'OAuth', value: value(props.summary?.oauth_status.normal), ariaLabel: t('admin.accounts.openaiSummary.normalAccounts'), active: statusActive('active'), filter: { type: 'oauth' as const, status: 'active' as const } }
    ]
  },
  {
    key: 'rate-limited',
    title: t('admin.accounts.openaiSummary.rateLimitedAccounts'),
    badge: t('admin.accounts.openaiSummary.rateLimited'),
    badgeClass: 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
    dotClass: 'bg-amber-500',
    active: statusActive('rate_limited'),
    items: [
      { key: 'status-rate-limited', label: 'OAuth', value: value(props.summary?.oauth_status.rate_limited), ariaLabel: t('admin.accounts.openaiSummary.rateLimitedAccounts'), active: statusActive('rate_limited'), filter: { type: 'oauth' as const, status: 'rate_limited' as const } }
    ]
  },
  {
    key: 'error',
    title: t('admin.accounts.openaiSummary.errorAccounts'),
    badge: t('admin.accounts.openaiSummary.error'),
    badgeClass: 'bg-rose-50 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300',
    dotClass: 'bg-rose-500',
    active: statusActive('error'),
    items: [
      { key: 'status-error', label: 'OAuth', value: value(props.summary?.oauth_status.error), ariaLabel: t('admin.accounts.openaiSummary.errorAccounts'), active: statusActive('error'), filter: { type: 'oauth' as const, status: 'error' as const } }
    ]
  },
  {
    key: 'quota-5h-card',
    title: t('admin.accounts.openaiSummary.quota5h'),
    badge: t('admin.accounts.openaiSummary.quota'),
    badgeClass: 'bg-indigo-50 text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-300',
    dotClass: 'bg-indigo-500',
    active: quotaActive('5h'),
    items: [
      { key: 'quota-5h', label: '', value: quota(props.summary?.oauth_quota['5h']), hint: t('admin.accounts.openaiSummary.remainingEstimated'), compact: true, ariaLabel: t('admin.accounts.openaiSummary.quota5h'), active: quotaActive('5h'), filter: { type: 'oauth' as const, quota_window: '5h' as const } }
    ]
  },
  {
    key: 'quota-7d-card',
    title: t('admin.accounts.openaiSummary.quota7d'),
    badge: t('admin.accounts.openaiSummary.quota'),
    badgeClass: 'bg-violet-50 text-violet-700 dark:bg-violet-900/30 dark:text-violet-300',
    dotClass: 'bg-violet-500',
    active: quotaActive('7d'),
    items: [
      { key: 'quota-7d', label: '', value: quota(props.summary?.oauth_quota['7d']), hint: t('admin.accounts.openaiSummary.remainingEstimated'), compact: true, ariaLabel: t('admin.accounts.openaiSummary.quota7d'), active: quotaActive('7d'), filter: { type: 'oauth' as const, quota_window: '7d' as const } }
    ]
  },
  {
    key: 'concurrency',
    title: t('admin.accounts.openaiSummary.concurrency'),
    badge: t('admin.accounts.openaiSummary.usedTotal'),
    badgeClass: 'bg-sky-50 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300',
    dotClass: 'bg-sky-500',
    active: typeActive('oauth') || typeActive('apikey'),
    items: [
      { key: 'concurrency-oauth', label: 'OAuth', value: concurrency(props.summary?.concurrency.oauth), ariaLabel: 'OAuth concurrency', active: typeActive('oauth'), filter: { type: 'oauth' as const } },
      { key: 'concurrency-apikey', label: 'APIKey', value: concurrency(props.summary?.concurrency.apikey), ariaLabel: 'APIKey concurrency', active: typeActive('apikey'), filter: { type: 'apikey' as const } }
    ]
  }
])
</script>

<style scoped>
.openai-summary-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 0.75rem;
}

@media (min-width: 640px) {
  .openai-summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (min-width: 1024px) {
  .openai-summary-grid {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
}

@media (min-width: 1280px) {
  .openai-summary-grid {
    grid-template-columns: 1.2fr repeat(3, 0.86fr) 1.2fr 1.2fr 1.35fr;
  }
}
</style>
