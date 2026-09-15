<template>
  <article
    class="border-b border-gray-100 last:border-b-0 dark:border-dark-700/70"
    :class="expanded ? accentBorderClass : ''"
    :data-testid="`monitor-row-${item.id}`"
  >
    <button
      type="button"
      class="group grid min-h-[72px] w-full grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-3 px-4 py-3 text-left outline-none transition-colors hover:bg-gray-50/80 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500 dark:hover:bg-dark-700/35 sm:grid-cols-[minmax(0,1fr)_auto_auto_auto] sm:px-5 lg:grid-cols-[minmax(230px,1.35fr)_110px_120px_minmax(280px,2fr)_44px] lg:gap-4"
      :aria-expanded="expanded"
      :aria-controls="panelId"
      @click="emit('toggle')"
    >
      <span class="flex min-w-0 items-center gap-3">
        <span class="grid h-9 w-9 shrink-0 place-items-center rounded-lg ring-1 ring-black/5 dark:ring-white/10" :class="[providerGradient(item.provider), providerTintClass]">
          <ProviderIcon :provider="item.provider" :size="20" />
        </span>
        <span class="min-w-0">
          <span class="block truncate text-sm font-semibold text-gray-900 dark:text-gray-100">{{ item.name }}</span>
          <span class="mt-0.5 flex min-w-0 items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400">
            <span class="shrink-0">{{ providerLabel(item.provider) }}</span>
            <span aria-hidden="true">·</span>
            <span class="truncate font-mono">{{ formatMonitorModel(item.primary_model) }}</span>
          </span>
        </span>
      </span>

      <span class="justify-self-end rounded-md px-2 py-1 text-xs font-semibold lg:justify-self-start" :class="statusClass">
        {{ statusLabel(item.primary_status) }}
      </span>

      <span class="hidden text-base font-semibold tabular-nums sm:block lg:justify-self-start" :class="availabilityClass">
        {{ formatPercent(availabilityValue) }}
      </span>

      <MonitorTimeline
        class="hidden lg:block"
        compact
        :buckets="item.timeline"
        :countdown-seconds="countdownSeconds"
      />

      <span class="grid h-11 w-11 place-items-center justify-self-end text-gray-400 transition-colors group-hover:text-gray-700 dark:group-hover:text-gray-200">
        <Icon name="chevronDown" size="sm" :class="['transition-transform duration-200', expanded ? 'rotate-180' : '']" />
      </span>
    </button>

    <div v-if="expanded" :id="panelId" class="bg-gray-50/65 px-4 pb-4 dark:bg-dark-900/25 sm:px-5" data-testid="monitor-expanded-panel">
      <div class="rounded-lg border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800 sm:p-5">
        <div class="grid gap-4 sm:grid-cols-3 sm:divide-x sm:divide-gray-200 dark:sm:divide-dark-700">
          <div>
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('monitorCommon.dialogLatency') }}</p>
            <p class="mt-1 text-xl font-semibold tabular-nums text-gray-900 dark:text-gray-100">{{ formatLatency(item.primary_latency_ms) }} <span class="text-xs font-normal text-gray-400">ms</span></p>
          </div>
          <div class="sm:pl-5">
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('monitorCommon.endpointPing') }}</p>
            <p class="mt-1 text-xl font-semibold tabular-nums text-gray-900 dark:text-gray-100">{{ formatLatency(item.primary_ping_latency_ms) }} <span class="text-xs font-normal text-gray-400">ms</span></p>
          </div>
          <div class="sm:pl-5">
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('channelStatus.details.checkCounts') }}</p>
            <p class="mt-1 text-sm font-semibold tabular-nums">
              <span class="text-emerald-600 dark:text-emerald-400">{{ timelineCounts.operational }} {{ t('monitorCommon.status.operational') }}</span>
              <span class="text-gray-300 dark:text-dark-600"> / </span>
              <span class="text-amber-600 dark:text-amber-400">{{ timelineCounts.degraded }} {{ t('monitorCommon.status.degraded') }}</span>
              <span class="text-gray-300 dark:text-dark-600"> / </span>
              <span class="text-red-600 dark:text-red-400">{{ timelineCounts.failed }} {{ t('monitorCommon.status.failed') }}</span>
            </p>
          </div>
        </div>

        <div class="mt-5 border-t border-gray-100 pt-4 dark:border-dark-700/70">
          <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
            <p class="text-xs font-semibold text-gray-600 dark:text-gray-300">{{ t('channelStatus.details.timeline') }}</p>
            <p class="text-xs font-semibold tabular-nums" :class="availabilityClass">{{ availabilityLabel }} · {{ formatPercent(availabilityValue) }}</p>
          </div>
          <MonitorTimeline :buckets="item.timeline" :countdown-seconds="countdownSeconds" />
          <div class="mt-3 flex flex-col gap-3 border-t border-gray-100 pt-3 text-xs dark:border-dark-700/70 sm:flex-row sm:items-center sm:justify-between">
            <p class="flex min-w-0 items-center gap-1.5 text-gray-500 dark:text-gray-400">
              <Icon :name="latestAbnormal ? 'exclamationCircle' : 'checkCircle'" size="sm" :class="latestAbnormal ? 'text-red-500' : 'text-emerald-500'" />
              <span v-if="latestAbnormal">{{ t('channelStatus.details.lastIncident') }}：{{ statusLabel(latestAbnormal.status) }} · {{ formatRelativeTime(latestAbnormal.checked_at) }}</span>
              <span v-else>{{ t('channelStatus.details.noIncident') }}</span>
            </p>
            <button type="button" class="min-h-11 self-start rounded-md px-3 text-sm font-medium text-primary-600 hover:bg-primary-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-primary-300 dark:hover:bg-primary-500/10 sm:self-auto" @click="emit('openDetail')">
              {{ t('channelStatus.actions.viewModels') }}
            </button>
          </div>
        </div>

        <MonitorQuotaView v-if="quotaVisible" :snapshot="item.latest_quota" class="mt-4 border-t border-gray-100 pt-4 dark:border-dark-700/70" />
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { UserMonitorView } from '@/api/channelMonitor'
import { useChannelMonitorFormat, providerGradient } from '@/composables/useChannelMonitorFormat'
import { isChannelMonitorQuotaVisible } from '@/utils/featureFlags'
import Icon from '@/components/icons/Icon.vue'
import MonitorQuotaView from '@/components/common/MonitorQuotaView.vue'
import ProviderIcon from './ProviderIcon.vue'
import MonitorTimeline from './MonitorTimeline.vue'

const PROVIDER_TINT: Record<string, string> = {
  openai: 'text-emerald-600 dark:text-emerald-300',
  anthropic: 'text-orange-600 dark:text-orange-300',
  gemini: 'text-sky-600 dark:text-sky-300',
  grok: 'text-zinc-700 dark:text-zinc-200',
  antigravity: 'text-purple-600 dark:text-purple-300',
  kimi: 'text-pink-600 dark:text-pink-300',
  zhipu: 'text-indigo-600 dark:text-indigo-300',
  deepseek: 'text-teal-600 dark:text-teal-300',
  opencode_go: 'text-amber-700 dark:text-amber-300',
}

const props = defineProps<{
  item: UserMonitorView
  window: '7d' | '15d' | '30d'
  availabilityValue: number | null
  countdownSeconds: number
  expanded: boolean
}>()

const emit = defineEmits<{ (e: 'toggle'): void; (e: 'openDetail'): void }>()
const { t } = useI18n()
const { statusLabel, providerLabel, formatLatency, formatMonitorModel, formatPercent, formatRelativeTime } = useChannelMonitorFormat()

const panelId = computed(() => `monitor-panel-${props.item.id}`)
const providerTintClass = computed(() => PROVIDER_TINT[props.item.provider] ?? 'text-gray-500 dark:text-gray-300')
const quotaVisible = computed(() => isChannelMonitorQuotaVisible() && !!props.item.latest_quota)
const availabilityLabel = computed(() => `${t('monitorCommon.availabilityPrefix')} · ${t(`channelStatus.windowTab.${props.window}`)}`)
const timelineCounts = computed(() => props.item.timeline.reduce((counts, point) => {
  if (point.status === 'operational') counts.operational += 1
  else if (point.status === 'degraded') counts.degraded += 1
  else counts.failed += 1
  return counts
}, { operational: 0, degraded: 0, failed: 0 }))
const latestAbnormal = computed(() => props.item.timeline.find(point => point.status !== 'operational'))

const severity = computed(() => props.item.primary_status === 'operational' ? 'operational' : props.item.primary_status === 'degraded' ? 'degraded' : 'failed')
const statusClass = computed(() => ({
  operational: 'bg-emerald-50 text-emerald-700 ring-1 ring-emerald-200 dark:bg-emerald-500/10 dark:text-emerald-300 dark:ring-emerald-500/30',
  degraded: 'bg-amber-50 text-amber-700 ring-1 ring-amber-200 dark:bg-amber-500/10 dark:text-amber-300 dark:ring-amber-500/30',
  failed: 'bg-red-50 text-red-700 ring-1 ring-red-200 dark:bg-red-500/10 dark:text-red-300 dark:ring-red-500/30',
}[severity.value]))
const availabilityClass = computed(() => ({
  operational: 'text-emerald-600 dark:text-emerald-400',
  degraded: 'text-amber-600 dark:text-amber-400',
  failed: 'text-red-600 dark:text-red-400',
}[severity.value]))
const accentBorderClass = computed(() => ({
  operational: 'border-l-[3px] border-l-emerald-500',
  degraded: 'border-l-[3px] border-l-amber-500',
  failed: 'border-l-[3px] border-l-red-500',
}[severity.value]))
</script>
