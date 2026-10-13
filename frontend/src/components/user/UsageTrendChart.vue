<template>
  <section class="usage-trend" :aria-label="t('usage.personal.trend')" :aria-busy="loading">
    <div class="usage-trend-heading">
      <div class="usage-trend-title">
        <h2>{{ t('usage.personal.trend') }} <span>{{ periodLabel }}</span></h2>
        <span class="usage-trend-legend"><span aria-hidden="true" />{{ metricLabel }}</span>
      </div>
      <div class="usage-trend-metrics" role="group" :aria-label="t('usage.personal.metric')">
        <button v-for="option in metricOptions" :key="option.value" type="button" :aria-pressed="metric === option.value" :class="{ selected: metric === option.value }" @click="metric = option.value">{{ option.label }}</button>
      </div>
    </div>
    <div class="usage-trend-plot">
      <div v-if="loading" class="usage-trend-state"><LoadingSpinner /></div>
      <div v-else-if="error" class="usage-trend-state" role="alert">
        {{ t('usage.failedToLoad') }}
        <button type="button" @click="$emit('retry')"><Icon name="refresh" size="sm" />{{ t('common.tryAgain') }}</button>
      </div>
      <template v-else-if="data.length">
        <Bar :data="chartData" :options="chartOptions" role="img" :aria-label="`${t('usage.personal.trend')}: ${periodLabel}, ${metricLabel}`" />
        <table class="sr-only">
          <caption>{{ metricLabel }}</caption>
          <thead><tr><th>{{ t('usage.time') }}</th><th>{{ metricLabel }}</th></tr></thead>
          <tbody><tr v-for="(slot, index) in slots" :key="slot"><td>{{ slot }}</td><td>{{ formatValue(values[index]) }}</td></tr></tbody>
        </table>
      </template>
      <div v-else class="usage-trend-state">{{ t('usage.personal.noActivity') }}</div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useDark } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { Chart as ChartJS, CategoryScale, LinearScale, BarElement, Tooltip, type ChartOptions } from 'chart.js'
import { Bar } from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import type { TrendDataPoint } from '@/types'

ChartJS.register(CategoryScale, LinearScale, BarElement, Tooltip)

const props = withDefaults(defineProps<{
  data: TrendDataPoint[]
  startDate: string
  endDate: string
  granularity: 'hour' | 'day'
  loading?: boolean
  error?: boolean
}>(), { loading: false, error: false })
defineEmits<{ retry: [] }>()
const { t, locale } = useI18n()
const isDark = useDark({ storageKey: 'theme' })
const metric = ref<'total_tokens' | 'requests' | 'actual_cost'>('total_tokens')
const metricOptions = computed(() => [
  { value: 'total_tokens' as const, label: 'Tokens' },
  { value: 'requests' as const, label: t('usage.personal.requests') },
  { value: 'actual_cost' as const, label: t('usage.cost') },
])
const metricLabel = computed(() => metricOptions.value.find(option => option.value === metric.value)!.label)
const periodLabel = computed(() => {
  if (props.startDate !== props.endDate) return `${props.startDate} ~ ${props.endDate}`
  const now = new Date()
  const today = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`
  return props.startDate === today ? t('dates.today') : props.startDate
})
const slots = computed(() => {
  const labels: string[] = []
  const date = new Date(`${props.startDate}T00:00:00Z`)
  const end = new Date(`${props.endDate}T00:00:00Z`)
  while (date <= end) {
    const day = date.toISOString().slice(0, 10)
    if (props.granularity === 'hour') {
      for (let hour = 0; hour < 24; hour++) labels.push(`${day} ${String(hour).padStart(2, '0')}:00`)
    } else labels.push(day)
    date.setUTCDate(date.getUTCDate() + 1)
  }
  return labels
})
const values = computed(() => {
  const bySlot = new Map<string, number>()
  for (const point of props.data) {
    const key = point.date.replace('T', ' ').slice(0, props.granularity === 'hour' ? 16 : 10)
    bySlot.set(key, (bySlot.get(key) || 0) + point[metric.value])
  }
  return slots.value.map(slot => bySlot.get(slot) || 0)
})
const formatValue = (value: number) => metric.value === 'actual_cost' ? `$${value.toFixed(4)}` : value.toLocaleString(locale.value)
const chartData = computed(() => ({
  labels: slots.value.map(slot => slot.slice(5).replace('-', '/')),
  datasets: [{ label: metricLabel.value, data: values.value, backgroundColor: '#4f7ee3', hoverBackgroundColor: '#3264d8', borderRadius: 3, maxBarThickness: 48, categoryPercentage: 0.85, barPercentage: 0.8 }],
}))
const chartOptions = computed<ChartOptions<'bar'>>(() => ({
  animation: false,
  responsive: true,
  maintainAspectRatio: false,
  interaction: { intersect: false, mode: 'index' },
  plugins: {
    legend: { display: false },
    tooltip: { displayColors: false, callbacks: {
      title: items => slots.value[items[0]?.dataIndex] || '',
      label: context => `${metricLabel.value}: ${formatValue(Number(context.raw))}`,
    } },
  },
  scales: {
    x: { grid: { display: false }, border: { display: false }, ticks: { color: isDark.value ? '#acacb6' : '#686874', maxRotation: 0, maxTicksLimit: 7, font: { size: 11 } } },
    y: { beginAtZero: true, border: { display: false }, grid: { color: isDark.value ? '#35353b' : '#e1e2e8' }, ticks: {
      color: isDark.value ? '#acacb6' : '#686874', maxTicksLimit: 3, font: { size: 11 },
      callback: value => `${metric.value === 'actual_cost' ? '$' : ''}${new Intl.NumberFormat(locale.value, { notation: 'compact', maximumFractionDigits: 2 }).format(Number(value))}`,
    } },
  },
}))
</script>

<style scoped>
.usage-trend { min-width: 0; padding: 14px 20px 16px; border: 1px solid var(--usage-line, #e1e2e8); border-radius: 8px; color: var(--usage-text, #17171d); background: var(--usage-surface, #fff); }
.usage-trend-heading, .usage-trend-title { display: flex; align-items: center; gap: 16px; min-width: 0; }
.usage-trend-heading { justify-content: space-between; flex-wrap: wrap; gap: 8px; }
.usage-trend-title { flex-wrap: wrap; gap: 6px 16px; }
h2 { margin: 0; font-size: 13px; line-height: 20px; font-weight: 600; }
h2 span { font-weight: 400; color: var(--usage-muted, #686874); }
.usage-trend-legend { display: inline-flex; align-items: center; gap: 6px; font-size: 13px; color: var(--usage-muted, #686874); }
.usage-trend-legend > span { width: 12px; height: 12px; border-radius: 2px; background: #4f7ee3; }
.usage-trend-metrics { display: flex; flex-shrink: 0; padding: 3px; border-radius: 7px; background: var(--usage-soft, #eeeef2); }
.usage-trend-metrics button { padding: 4px 12px; border-radius: 5px; font-size: 12px; line-height: 20px; color: var(--usage-muted, #686874); }
.usage-trend-metrics .selected { font-weight: 600; background: var(--usage-surface, #fff); color: var(--usage-text, #17171d); box-shadow: 0 1px 3px #0000000d; }
.usage-trend-plot { position: relative; height: 164px; margin-top: 8px; }
.usage-trend-state { display: flex; align-items: center; justify-content: center; gap: 12px; height: 100%; font-size: 13px; color: var(--usage-muted, #686874); }
.usage-trend-state button { display: inline-flex; align-items: center; gap: 5px; text-decoration: underline; }
@media (max-width: 640px) {
  .usage-trend { padding: 14px 12px; }
  .usage-trend-metrics button { min-width: 44px; min-height: 44px; }
}
</style>
