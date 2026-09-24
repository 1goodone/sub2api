<template>
  <AppLayout class="console-signal">
    <div class="signal-channel-page signal-channel-v3 space-y-5 pb-12">
      <section class="signal-v3-toolbar">
        <header class="signal-v3-heading flex items-start justify-between gap-4">
          <div class="min-w-0">
            <h1 class="page-title text-2xl font-semibold text-gray-900 dark:text-white">
              {{ t('channelMonitorV3.title') }}
            </h1>
            <div class="signal-v3-updated mt-2 flex flex-wrap items-center gap-2 text-xs">
              <span class="h-2 w-2 rounded-full" :class="refreshing ? 'bg-gray-400' : 'bg-emerald-500'" />
              <span>{{ snapshot ? t('channelMonitorV3.updatedTo', { time: formatTime(snapshot.coverage.data_through) }) : t('common.loading') }}</span>
              <span v-if="snapshot && !snapshot.coverage.coverage_complete" class="badge badge-warning">{{ t('channelMonitorV3.partialCoverage') }}</span>
            </div>
          </div>
          <button class="btn btn-secondary btn-icon h-11 w-11" type="button" :disabled="loading || refreshing" :aria-label="t('common.refresh')" :title="t('common.refresh')" @click="reload(false)"><Icon name="refresh" size="sm" :class="refreshing ? 'animate-spin' : ''" /></button>
        </header>
        <div class="signal-v3-controls">
          <div class="signal-v3-ranges" role="group" :aria-label="t('channelMonitorV2.timeRange')">
            <button v-for="option in ranges" :key="option.value" type="button" :aria-pressed="filter.range === option.value" @click="setRange(option.value)">{{ option.label }}</button>
          </div>
          <dl v-if="snapshot" class="signal-v3-summary">
            <div><dt>{{ t('channelMonitorV3.successRate') }}</dt><dd>{{ formatPercent(1 - (latestSnapshotMetrics?.error_rate ?? 0)) }}</dd></div>
            <div><dt>{{ t('channelMonitorV3.cacheRate') }}</dt><dd>{{ formatPercent(snapshot.metrics.cache_rate) }}</dd></div>
          </dl>
        </div>
      </section>

      <div v-if="loading && rows.length === 0" class="signal-v3-card-grid signal-v3-loading">
        <div v-for="i in 4" :key="i" class="h-72 animate-pulse bg-white/60 dark:bg-dark-800" />
      </div>
      <EmptyState v-else-if="rows.length === 0" :title="t('channelMonitorV3.emptyTitle')" :description="t('channelMonitorV3.emptyDescription')" />
      <div v-else class="channel-status-board" data-testid="channel-status-board">
        <section
          v-for="(block, index) in layoutBlocks"
          :key="channelStatusLayoutBlockKey(block, index)"
          :data-layout="block.kind"
          :data-testid="block.kind === 'cluster' ? `channel-status-platform-${block.platform}` : 'channel-status-compact-platforms'"
        >
          <template v-if="block.kind === 'cluster'">
            <h2 class="signal-platform-heading mb-3 flex items-center gap-2 px-1 text-sm font-semibold text-gray-700 dark:text-gray-200">
              {{ providerLabel(block.platform) }}
              <span class="rounded-full bg-gray-100 px-1.5 py-px font-mono text-[10px] font-medium text-gray-500 dark:bg-dark-700 dark:text-gray-400">{{ block.rows.length }}</span>
            </h2>
            <div class="signal-v3-card-grid">
              <ChannelMonitorV3Card
                v-for="row in block.rows"
                :key="row.group_id ?? `${row.platform}:${row.group_name ?? ''}`"
                :row="row"
                :show-platform-badge="false"
                :user-rate-multiplier="getUserRateMultiplier(row.group_id)"
                :coverage="matrix?.coverage ?? snapshot?.coverage"
                :countdown-seconds="countdownSeconds"
                :timeline-length="timelineLength"
              />
            </div>
          </template>
          <div
            v-else
            class="signal-v3-card-grid"
          >
            <div
              v-for="item in block.items"
              :key="item.row.group_id ?? `${item.platform}:${item.row.group_name ?? ''}`"
              :data-testid="`channel-status-platform-${item.platform}`"
            >
              <h2 class="signal-platform-heading mb-3 px-1 text-sm font-semibold text-gray-700 dark:text-gray-200">{{ providerLabel(item.platform) }}</h2>
              <ChannelMonitorV3Card
                :row="item.row"
                :show-platform-badge="false"
                :user-rate-multiplier="getUserRateMultiplier(item.row.group_id)"
                :coverage="matrix?.coverage ?? snapshot?.coverage"
                :countdown-seconds="countdownSeconds"
                :timeline-length="timelineLength"
              />
            </div>
          </div>
        </section>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import '@/styles/console-signal.css'
import '@/styles/console-studio.css'
import './channel-signal.css'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import * as api from '@/api/channelMonitorV2'
import userGroupsAPI from '@/api/groups'
import type { MonitorFilter, MonitorMatrixResponse, MonitorRange, MonitorSnapshot } from '@/api/channelMonitorV2'
import type { Group } from '@/types'
import ChannelMonitorV3Card from '@/components/user/monitor/ChannelMonitorV3Card.vue'
import { useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'
import { formatMonitorPercent } from '@/features/channel-monitor-v2/monitorFormat'
import { buildChannelStatusLayout, channelStatusLayoutBlockKey } from '@/features/channel-monitor-v2/channelStatusLayout'

const { t, locale } = useI18n()
const appStore = useAppStore()
const { providerLabel } = useChannelMonitorFormat()
const ranges = computed(() => [
  { value: '90m' as MonitorRange, label: t('channelMonitorV3.ranges.90m') },
  { value: '24h' as MonitorRange, label: t('channelMonitorV3.ranges.24h') },
  { value: '7d' as MonitorRange, label: t('channelMonitorV3.ranges.7d') },
  { value: '30d' as MonitorRange, label: t('channelMonitorV3.ranges.30d') },
])
const filter = ref<MonitorFilter>({ range: '90m', platforms: [], groupIds: [], models: [] })
const snapshot = ref<MonitorSnapshot | null>(null)
const matrix = ref<MonitorMatrixResponse | null>(null)
const loading = ref(false)
const refreshing = ref(false)
const userGroupRates = ref<Record<number, number>>({})
const countdownSeconds = ref(0)
let controller: AbortController | null = null
let refreshTimer: number | null = null
let countdownTimer: number | null = null

// platform_group is intentionally used here: the backend scopes it to the
// monitor group_ids selected by the operator, so unrelated groups never appear.
const rows = computed(() => [...(matrix.value?.items ?? [])]
  .filter(row => row.group_id != null && row.group_id > 0)
  .sort((a, b) => (a.group_id ?? 0) - (b.group_id ?? 0)))
const platformSections = computed(() => {
  const grouped = new Map<string, typeof rows.value>()
  for (const row of rows.value) {
    const platform = row.platform || 'unknown'
    const current = grouped.get(platform) ?? []
    current.push(row)
    grouped.set(platform, current)
  }
  return [...grouped.entries()].map(([platform, sectionRows]) => ({
    platform,
    rows: sectionRows.sort((a, b) => (a.group_id ?? 0) - (b.group_id ?? 0)),
  }))
})
const layoutBlocks = computed(() => buildChannelStatusLayout(platformSections.value))
const timelineLength = computed(() => ({ '90m': 18, '24h': 24, '7d': 14, '30d': 30 })[filter.value.range])
const latestSnapshotMetrics = computed(() => {
  const trend = [...(snapshot.value?.trend ?? [])]
    .filter(point => point.bucket_start && point.metrics)
    .sort((a, b) => Date.parse(a.bucket_start) - Date.parse(b.bucket_start))
  return trend.at(-1)?.metrics ?? snapshot.value?.metrics
})

function formatPercent(value: number) { return formatMonitorPercent(value, locale.value || 'zh-CN') }
function formatTime(value?: string) { if (!value) return '-'; return new Intl.DateTimeFormat(locale.value || undefined, { dateStyle: 'short', timeStyle: 'short' }).format(new Date(value)) }
function setRange(value: MonitorRange) { filter.value = { ...filter.value, range: value } }

function getUserRateMultiplier(groupId?: number) {
  if (!groupId) return null
  const value = userGroupRates.value[groupId]
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

async function loadUserGroupRates() {
  try {
    const [groups, customRates] = await Promise.all([
      userGroupsAPI.getAvailable(),
      userGroupsAPI.getUserGroupRates(),
    ])
    userGroupRates.value = Object.fromEntries(
      (groups as Group[]).map(group => [group.id, customRates[group.id] ?? group.rate_multiplier]),
    )
  } catch (error) {
    // Monitoring remains usable when the optional pricing endpoint is unavailable.
    console.warn('Failed to load channel monitor group rates', error)
  }
}

async function reload(silent = true) {
  controller?.abort()
  const request = new AbortController()
  controller = request
  refreshing.value = true
  if (!silent) loading.value = true
  try {
    const [nextSnapshot, nextMatrix] = await Promise.all([
      api.getSnapshot(filter.value, false, request.signal),
      api.getMatrix(filter.value, 'platform_group', false, request.signal),
    ])
    if (request.signal.aborted || controller !== request) return
    snapshot.value = nextSnapshot
    matrix.value = nextMatrix
    scheduleRefresh(nextSnapshot.coverage.bootstrap?.active ? 10 : nextSnapshot.config.refresh_interval_seconds)
  } catch (error) {
    const e = error as { name?: string; code?: string }
    if (e.name !== 'AbortError' && e.code !== 'ERR_CANCELED') appStore.showError(extractApiErrorMessage(error, t('channelMonitorV3.loadFailed')))
  } finally {
    if (controller === request) { loading.value = false; refreshing.value = false }
  }
}

function scheduleRefresh(seconds: number) {
  const interval = Math.max(10, seconds)
  if (refreshTimer) window.clearInterval(refreshTimer)
  if (countdownTimer) window.clearInterval(countdownTimer)
  countdownSeconds.value = interval
  countdownTimer = window.setInterval(() => {
    if (!document.hidden && countdownSeconds.value > 0) countdownSeconds.value -= 1
  }, 1000)
  refreshTimer = window.setInterval(() => {
    if (!loading.value && !refreshing.value && !document.hidden) void reload(true)
  }, interval * 1000)
}

watch(() => filter.value.range, () => void reload(false))
onMounted(() => { void loadUserGroupRates(); void reload(false) })
onBeforeUnmount(() => {
  controller?.abort()
  if (refreshTimer) window.clearInterval(refreshTimer)
  if (countdownTimer) window.clearInterval(countdownTimer)
})
</script>
