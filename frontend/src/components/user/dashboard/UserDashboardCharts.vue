<template>
  <div class="space-y-6">
    <!-- Date Range Filter -->
    <div class="dashboard-chart-filters">
      <div class="flex flex-wrap items-center gap-4">
        <div class="flex items-center gap-2">
          <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('dashboard.timeRange') }}:</span>
          <DateRangePicker :start-date="startDate" :end-date="endDate" @update:startDate="$emit('update:startDate', $event)" @update:endDate="$emit('update:endDate', $event)" @change="$emit('dateRangeChange', $event)" />
        </div>
        <button @click="$emit('refresh')" :disabled="loading" class="btn btn-secondary">
          <Icon name="refresh" size="sm" />
          {{ t('common.refresh') }}
        </button>
        <div class="ml-auto flex items-center gap-2">
          <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('dashboard.granularity') }}:</span>
          <div class="w-28">
            <Select :model-value="granularity" :options="[{value:'day', label:t('dashboard.day')}, {value:'hour', label:t('dashboard.hour')}]" @update:model-value="$emit('update:granularity', $event)" @change="$emit('granularityChange')" />
          </div>
        </div>
      </div>
    </div>

    <!-- Charts Grid -->
    <div class="dashboard-charts-grid grid grid-cols-1 gap-6">
      <!-- Model Distribution Chart -->
      <div class="card dashboard-models relative overflow-hidden p-4">
        <div v-if="loading" class="absolute inset-0 z-10 flex items-center justify-center bg-white/50 backdrop-blur-sm dark:bg-dark-800/50">
          <LoadingSpinner size="md" />
        </div>
        <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">{{ t('dashboard.modelDistribution') }}</h3>
        <div class="dashboard-model-content flex flex-col items-center gap-4">
          <div v-if="!models.length" class="py-12 text-sm text-gray-500">{{ t('dashboard.noDataAvailable') }}</div>
          <div v-else class="model-share-list">
            <div v-for="(model, index) in models" :key="model.model" class="model-share-item">
              <div class="model-share-heading"><span>{{ model.model }}</span><strong>{{ modelShare(model.total_tokens).toFixed(1) }}<small>%</small></strong></div>
              <div class="model-share-track" role="meter" :aria-label="`${model.model} Token`" :aria-valuenow="modelShare(model.total_tokens)" :aria-valuemin="0" :aria-valuemax="100"><span :style="{ width: `${modelShare(model.total_tokens)}%`, background: modelColors[index % modelColors.length] }" /></div>
              <div class="model-share-meta"><span>{{ formatNumber(model.requests) }} {{ t('dashboard.requests') }} · {{ formatTokens(model.total_tokens) }} Token</span><span :title="t('dashboard.actual')">${{ formatCost(model.actual_cost) }}</span></div>
            </div>
          </div>
          <details v-if="models.length" class="model-details w-full min-w-0">
            <summary>{{ t('dashboard.standard') }} / {{ t('dashboard.actual') }}<Icon name="chevronDown" size="sm" /></summary>
          <div class="max-h-48 w-full min-w-0 overflow-auto">
            <table class="w-full text-xs">
              <thead>
                <tr class="text-gray-500 dark:text-gray-400">
                  <th class="pb-2 text-left">{{ t('dashboard.model') }}</th>
                  <th class="pb-2 text-right">{{ t('dashboard.requests') }}</th>
                  <th class="pb-2 text-right">{{ t('dashboard.tokens') }}</th>
                  <th class="pb-2 text-right">{{ t('dashboard.actual') }}</th>
                  <th class="pb-2 text-right">{{ t('dashboard.standard') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(model, index) in models" :key="model.model" class="border-t border-gray-100 dark:border-dark-700">
                  <td class="max-w-[100px] truncate py-1.5 font-medium text-gray-900 dark:text-white" :title="model.model"><span class="model-swatch" :style="{ background: modelColors[index % modelColors.length] }" />{{ model.model }}</td>
                  <td class="py-1.5 text-right text-gray-600 dark:text-gray-400">{{ formatNumber(model.requests) }}</td>
                  <td class="py-1.5 text-right text-gray-600 dark:text-gray-400">{{ formatTokens(model.total_tokens) }}</td>
                  <td class="py-1.5 text-right text-gray-900 dark:text-white">${{ formatCost(model.actual_cost) }}</td>
                  <td class="py-1.5 text-right text-gray-400 dark:text-gray-500">${{ formatCost(model.cost) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          </details>
        </div>
      </div>

      <!-- Token Usage Trend Chart -->
      <TokenUsageTrend :trend-data="trend" :loading="loading" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Select from '@/components/common/Select.vue'
import TokenUsageTrend from '@/components/charts/TokenUsageTrend.vue'
import type { TrendDataPoint, ModelStat } from '@/types'
import { formatCostFixed as formatCost, formatNumberLocaleString as formatNumber, formatTokensK as formatTokens } from '@/utils/format'

const props = defineProps<{ loading: boolean, startDate: string, endDate: string, granularity: string, trend: TrendDataPoint[], models: ModelStat[] }>()
defineEmits(['update:startDate', 'update:endDate', 'update:granularity', 'dateRangeChange', 'granularityChange', 'refresh'])
const { t } = useI18n()
const modelColors = ['#4f7ee3', '#72b3aa', '#c9a568', '#b58090', '#8a90be', '#7297ad', '#999a7b', '#ab8870']

const totalModelTokens = computed(() => props.models.reduce((total, model) => total + model.total_tokens, 0))
const modelShare = (tokens: number) => totalModelTokens.value > 0 ? tokens / totalModelTokens.value * 100 : 0
</script>
