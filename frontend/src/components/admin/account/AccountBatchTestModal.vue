<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.batchTest.title')"
    width="wide"
    @close="handleClose"
  >
    <div class="space-y-4">
      <div class="flex flex-wrap items-end justify-between gap-3" aria-live="polite">
        <div>
          <div class="text-2xl font-semibold text-gray-900 dark:text-gray-100">
            {{ completedCount }} / {{ items.length }}
          </div>
          <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.batchTest.progress') }}
          </div>
        </div>
        <div class="flex flex-wrap gap-x-4 gap-y-1 text-sm">
          <span class="text-green-600 dark:text-green-400">
            {{ t('admin.accounts.batchTest.successCount', { count: successCount }) }}
          </span>
          <span class="text-red-600 dark:text-red-400">
            {{ t('admin.accounts.batchTest.failedCount', { count: failedCount }) }}
          </span>
        </div>
      </div>

      <div
        class="h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-600"
        role="progressbar"
        :aria-valuemin="0"
        :aria-valuemax="items.length"
        :aria-valuenow="completedCount"
      >
        <div
          class="h-full bg-primary-500 transition-[width] duration-200"
          :style="{ width: `${progressPercent}%` }"
        ></div>
      </div>

      <div class="max-h-[50vh] overflow-y-auto border-y border-gray-200 dark:border-dark-500">
        <div
          v-for="item in items"
          :key="item.id"
          :data-testid="`batch-test-row-${item.id}`"
          class="grid min-h-14 grid-cols-[minmax(0,1fr)_7.5rem] items-center gap-3 border-b border-gray-100 px-1 py-3 last:border-b-0 dark:border-dark-600 sm:grid-cols-[minmax(0,1fr)_8rem_8rem]"
        >
          <div class="min-w-0">
            <div class="truncate text-sm font-medium text-gray-900 dark:text-gray-100" :title="item.name">
              {{ item.name }}
            </div>
            <div class="mt-0.5 flex min-w-0 items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
              <span>#{{ item.id }}</span>
              <span v-if="item.platform" class="truncate">{{ item.platform }}</span>
              <span v-if="item.error" class="truncate text-red-600 dark:text-red-400" :title="item.error">
                {{ item.error }}
              </span>
            </div>
          </div>

          <div class="hidden min-w-0 text-xs text-gray-500 dark:text-gray-400 sm:block">
            <div v-if="item.model" class="truncate" :title="item.model">{{ item.model }}</div>
            <div v-if="item.durationMs !== undefined" class="mt-0.5">{{ item.durationMs }} ms</div>
          </div>

          <div class="flex items-center justify-end gap-2 text-sm font-medium" :class="statusClass(item.status)">
            <Icon
              :name="statusIcon(item.status)"
              size="sm"
              :class="item.status === 'running' ? 'animate-spin' : ''"
              :stroke-width="2"
            />
            <span>{{ statusLabel(item.status) }}</span>
          </div>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex w-full flex-wrap justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="handleClose">
          {{ t('common.close') }}
        </button>
        <button
          v-if="isRunning"
          type="button"
          class="btn btn-warning inline-flex items-center gap-2"
          data-testid="stop-batch-test"
          @click="stopTests"
        >
          <Icon name="x" size="sm" :stroke-width="2" />
          {{ t('admin.accounts.batchTest.stop') }}
        </button>
        <button
          v-else
          type="button"
          class="btn btn-primary inline-flex items-center gap-2"
          :disabled="items.length === 0"
          data-testid="start-batch-test"
          @click="startTests"
        >
          <Icon :name="hasStarted ? 'refresh' : 'play'" size="sm" :stroke-width="2" />
          {{ hasStarted ? t('admin.accounts.batchTest.retry') : t('admin.accounts.batchTest.start') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import type { Account } from '@/types'

type BatchTestStatus = 'pending' | 'running' | 'success' | 'failed' | 'cancelled'

interface BatchTestItem {
  id: number
  name: string
  platform?: string
  status: BatchTestStatus
  model?: string
  error?: string
  durationMs?: number
}

const props = defineProps<{
  show: boolean
  accountIds: number[]
  accounts: Account[]
}>()

const emit = defineEmits<{
  (event: 'close'): void
}>()

const { t } = useI18n()
const items = ref<BatchTestItem[]>([])
const isRunning = ref(false)
const hasStarted = ref(false)
let activeController: AbortController | null = null
let runVersion = 0

const completedCount = computed(() => items.value.filter(item => item.status !== 'pending' && item.status !== 'running').length)
const successCount = computed(() => items.value.filter(item => item.status === 'success').length)
const failedCount = computed(() => items.value.filter(item => item.status === 'failed').length)
const progressPercent = computed(() => items.value.length ? (completedCount.value / items.value.length) * 100 : 0)

const resetItems = () => {
  const accountByID = new Map(props.accounts.map(account => [account.id, account]))
  items.value = props.accountIds.map(id => {
    const account = accountByID.get(id)
    return {
      id,
      name: account?.name || t('admin.accounts.batchTest.accountFallback', { id }),
      platform: account?.platform,
      status: 'pending'
    }
  })
  hasStarted.value = false
}

const startTests = async () => {
  if (isRunning.value || items.value.length === 0) return

  const version = ++runVersion
  items.value.forEach(item => {
    item.status = 'pending'
    item.model = undefined
    item.error = undefined
    item.durationMs = undefined
  })
  hasStarted.value = true
  isRunning.value = true

  for (const item of items.value) {
    if (version !== runVersion) return
    item.status = 'running'
    const startedAt = performance.now()
    activeController = new AbortController()

    try {
      const result = await adminAPI.accounts.testAccount(item.id, { prompt: 'hi' }, {
        signal: activeController.signal,
        onEvent: event => {
          if (event.model) item.model = event.model
        }
      })
      if (version !== runVersion) return
      item.status = result.success ? 'success' : 'failed'
      item.model = result.model || item.model
      item.error = result.success ? undefined : result.error || t('admin.accounts.testFailed')
    } catch (error) {
      if (version !== runVersion) return
      item.status = 'failed'
      item.error = error instanceof Error ? error.message : t('common.unknownError')
    } finally {
      if (version === runVersion) item.durationMs = Math.round(performance.now() - startedAt)
    }
  }

  if (version === runVersion) {
    activeController = null
    isRunning.value = false
  }
}

const stopTests = () => {
  if (!isRunning.value) return
  runVersion++
  activeController?.abort()
  activeController = null
  items.value.forEach(item => {
    if (item.status === 'pending' || item.status === 'running') item.status = 'cancelled'
  })
  isRunning.value = false
}

const handleClose = () => {
  stopTests()
  emit('close')
}

const statusIcon = (status: BatchTestStatus) => ({
  pending: 'clock',
  running: 'refresh',
  success: 'checkCircle',
  failed: 'xCircle',
  cancelled: 'x'
} as const)[status]

const statusClass = (status: BatchTestStatus) => ({
  pending: 'text-gray-500 dark:text-gray-400',
  running: 'text-amber-600 dark:text-amber-400',
  success: 'text-green-600 dark:text-green-400',
  failed: 'text-red-600 dark:text-red-400',
  cancelled: 'text-gray-500 dark:text-gray-400'
})[status]

const statusLabel = (status: BatchTestStatus) => t(`admin.accounts.batchTest.status.${status}`)

watch(() => props.show, show => {
  if (show) resetItems()
  else stopTests()
}, { immediate: true })

onUnmounted(stopTests)
</script>
