<template>
  <section class="card overflow-hidden" aria-labelledby="cloudcat-merchant-title">
    <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-100 px-4 py-3 dark:border-dark-700">
      <div class="flex items-center gap-3">
        <div class="rounded-lg bg-sky-100 p-2 dark:bg-sky-900/30">
          <Icon name="cloud" size="md" class="text-sky-600 dark:text-sky-400" :stroke-width="2" />
        </div>
        <div>
          <h2 id="cloudcat-merchant-title" class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('payment.admin.cloudcat.title') }}
          </h2>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.cloudcat.subtitle') }}</p>
        </div>
      </div>
      <div class="flex items-center gap-2">
        <span
          v-if="overview?.connected"
          class="inline-flex items-center gap-1.5 rounded-full bg-emerald-50 px-2.5 py-1 text-xs font-medium text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300"
        >
          <span class="h-1.5 w-1.5 rounded-full bg-emerald-500" aria-hidden="true"></span>
          {{ t('payment.admin.cloudcat.connected') }}
        </span>
        <span
          v-else-if="overview?.needs_login"
          class="inline-flex items-center gap-1.5 rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300"
        >
          {{ t('payment.admin.cloudcat.notConnected') }}
        </span>
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          :disabled="loading || submitting"
          :title="t('common.refresh')"
          :aria-label="t('common.refresh')"
          @click="loadOverview(true)"
        >
          <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
        </button>
      </div>
    </div>

    <div v-if="initialLoading" class="flex items-center justify-center py-10">
      <LoadingSpinner size="sm" />
    </div>

    <div v-else-if="overview?.connected" class="p-4">
      <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <div class="rounded-lg border border-gray-100 p-3 dark:border-dark-700">
          <div class="mb-2 flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
            <Icon name="chart" size="sm" />
            {{ t('payment.admin.cloudcat.todayOrders') }}
          </div>
          <p class="text-xl font-semibold text-gray-900 dark:text-white">{{ overview.today_order_count }}</p>
        </div>
        <div class="rounded-lg border border-gray-100 p-3 dark:border-dark-700">
          <div class="mb-2 flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
            <Icon name="dollar" size="sm" />
            {{ t('payment.admin.cloudcat.todayAmount') }}
          </div>
          <p class="text-xl font-semibold text-gray-900 dark:text-white">{{ formatMoney(overview.today_order_amount) }}</p>
        </div>
        <div class="rounded-lg border border-gray-100 p-3 dark:border-dark-700">
          <div class="mb-2 flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
            <Icon name="creditCard" size="sm" />
            {{ t('payment.admin.cloudcat.withdrawable') }}
          </div>
          <p class="text-xl font-semibold text-gray-900 dark:text-white">{{ formatMoney(overview.withdrawable_amount) }}</p>
        </div>
        <div class="rounded-lg border border-gray-100 p-3 dark:border-dark-700">
          <div class="mb-2 flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
            <Icon name="lock" size="sm" />
            {{ t('payment.admin.cloudcat.frozen') }}
          </div>
          <p class="text-xl font-semibold text-gray-900 dark:text-white">{{ formatMoney(overview.frozen_amount) }}</p>
        </div>
      </div>
      <div class="mt-4 flex flex-wrap items-center justify-between gap-3 text-xs text-gray-500 dark:text-gray-400">
        <span>{{ t('payment.admin.cloudcat.lastSynced', { time: lastSyncedLabel }) }}</span>
        <button type="button" class="btn btn-secondary btn-sm" :disabled="loading || submitting" @click="logout">
          <Icon name="login" size="sm" class="rotate-180" />
          {{ t('payment.admin.cloudcat.logout') }}
        </button>
      </div>
    </div>

    <form v-else-if="showLoginForm" class="space-y-4 p-4" @submit.prevent="login">
      <div class="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200">
        {{ t('payment.admin.cloudcat.loginHint') }}
      </div>
      <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
        <label class="block">
          <span class="input-label">{{ t('payment.admin.cloudcat.username') }}</span>
          <input v-model.trim="username" type="text" autocomplete="username" class="input" :disabled="submitting" required />
        </label>
        <label class="block">
          <span class="input-label">{{ t('payment.admin.cloudcat.password') }}</span>
          <input v-model="password" type="password" autocomplete="current-password" class="input" :disabled="submitting" required />
        </label>
        <label class="block">
          <span class="input-label">{{ t('payment.admin.cloudcat.safeCode') }}</span>
          <input v-model.trim="safeCode" type="text" autocomplete="one-time-code" class="input" :disabled="submitting" />
        </label>
        <label class="block">
          <span class="input-label">{{ t('payment.admin.cloudcat.emailCode') }}</span>
          <input v-model.trim="emailCode" type="text" autocomplete="one-time-code" class="input" :disabled="submitting" />
        </label>
      </div>
      <div class="flex items-center justify-end">
        <button type="submit" class="btn btn-primary" :disabled="submitting || !username || !password">
          <LoadingSpinner v-if="submitting" size="sm" color="white" />
          <Icon v-else name="login" size="sm" />
          {{ t('payment.admin.cloudcat.login') }}
        </button>
      </div>
    </form>

    <div v-else class="flex items-center justify-between gap-3 p-4 text-sm text-gray-600 dark:text-gray-300">
      <span>{{ t('payment.admin.cloudcat.loadFailed') }}</span>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="loadOverview(true)">
        <Icon name="refresh" size="sm" />
        {{ t('payment.admin.retry') }}
      </button>
    </div>

    <p v-if="errorMessage" class="border-t border-red-100 px-4 py-3 text-xs text-red-600 dark:border-red-900/40 dark:text-red-400" role="alert">
      {{ errorMessage }}
    </p>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminPaymentAPI } from '@/api/admin/payment'
import { extractI18nErrorMessage } from '@/utils/apiError'
import type { CloudcatMerchantOverview } from '@/types/payment'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'

const { t } = useI18n()
const appStore = useAppStore()

const overview = ref<CloudcatMerchantOverview | null>(null)
const username = ref('')
const password = ref('')
const safeCode = ref('')
const emailCode = ref('')
const initialLoading = ref(true)
const loading = ref(false)
const submitting = ref(false)
const errorMessage = ref('')
let refreshTimer: ReturnType<typeof setInterval> | null = null

const showLoginForm = computed(() => overview.value?.needs_login === true || (!overview.value && !errorMessage.value))

const lastSyncedLabel = computed(() => {
  if (!overview.value?.last_synced_at) return t('payment.admin.cloudcat.notSynced')
  const date = new Date(overview.value.last_synced_at)
  return Number.isNaN(date.getTime()) ? t('payment.admin.cloudcat.notSynced') : date.toLocaleString()
})

function formatMoney(amount: number): string {
  const currency = overview.value?.currency || 'CNY'
  try {
    return new Intl.NumberFormat(undefined, { style: 'currency', currency }).format(amount)
  } catch {
    return `${currency} ${amount.toFixed(2)}`
  }
}

async function loadOverview(showError: boolean) {
  if (loading.value || submitting.value) return
  loading.value = true
  errorMessage.value = ''
  try {
    const res = await adminPaymentAPI.getCloudcatOverview()
    overview.value = res.data
  } catch (err: unknown) {
    errorMessage.value = extractI18nErrorMessage(err, t, 'payment.admin.cloudcat.errors', t('payment.admin.cloudcat.loadFailed'))
    if (showError) appStore.showError(errorMessage.value)
  } finally {
    loading.value = false
    initialLoading.value = false
  }
}

async function login() {
  if (!username.value || !password.value || submitting.value) return
  submitting.value = true
  errorMessage.value = ''
  try {
    const res = await adminPaymentAPI.loginCloudcat({
      username: username.value,
      password: password.value,
      ...(safeCode.value ? { safecode: safeCode.value } : {}),
      ...(emailCode.value ? { email_code: emailCode.value } : {}),
    })
    overview.value = res.data
    safeCode.value = ''
    emailCode.value = ''
  } catch (err: unknown) {
    errorMessage.value = extractI18nErrorMessage(err, t, 'payment.admin.cloudcat.errors', t('payment.admin.cloudcat.loginFailed'))
    appStore.showError(errorMessage.value)
  } finally {
    password.value = ''
    submitting.value = false
  }
}

async function logout() {
  if (loading.value || submitting.value) return
  loading.value = true
  errorMessage.value = ''
  try {
    await adminPaymentAPI.logoutCloudcat()
    overview.value = { connected: false, needs_login: true, today_order_count: 0, today_order_amount: 0, withdrawable_amount: 0, frozen_amount: 0, currency: 'CNY' }
  } catch (err: unknown) {
    errorMessage.value = extractI18nErrorMessage(err, t, 'payment.admin.cloudcat.errors', t('payment.admin.cloudcat.logoutFailed'))
    appStore.showError(errorMessage.value)
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await loadOverview(false)
  refreshTimer = setInterval(() => {
    if (overview.value?.connected) void loadOverview(false)
  }, 5 * 60 * 1000)
})

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>
