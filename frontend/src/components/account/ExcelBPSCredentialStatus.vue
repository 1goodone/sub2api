<template>
  <div v-if="state" class="space-y-1 text-xs" data-testid="excel-credential-state" aria-live="polite">
    <div class="flex flex-wrap items-center gap-1.5">
      <span :class="failed ? 'text-red-600 dark:text-red-400' : 'text-gray-500 dark:text-gray-400'">{{ t(`admin.accounts.bpsCredentialState.${status}`) }}</span>
      <button
        v-if="canReauthorize"
        type="button"
        data-testid="excel-reauthorize"
        class="rounded border border-amber-300 px-1.5 py-0.5 text-[11px] font-medium leading-4 text-amber-700 hover:bg-amber-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-amber-700 dark:text-amber-300 dark:hover:bg-amber-950/40"
        :disabled="starting"
        @click="reauthorize"
      >
        {{ starting ? t('admin.accounts.bpsCredentialState.reauthorizing') : t('admin.accounts.bpsCredentialState.reauthorize') }}
      </button>
    </div>
    <p v-if="notice" :class="noticeError ? 'text-red-600 dark:text-red-400' : 'text-emerald-600 dark:text-emerald-400'">{{ notice }}</p>
    <p v-if="state.expires_at" class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.bpsCredentialState.expiresAt') }}: {{ formatDateTime(state.expires_at) }}</p>
    <p v-if="state.error_code" class="font-mono text-gray-500 dark:text-gray-400">{{ state.error_code }}</p>
    <p v-if="state.requires_manual_resume" class="text-amber-600 dark:text-amber-400">{{ t('admin.accounts.bpsCredentialState.manualResume') }}</p>
  </div>
</template>
<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { ExcelBPSCredentialState } from '@/types'
import { formatDateTime } from '@/utils/format'
import { excelCredentialStatus, useExcelCredentialClock } from '@/utils/excelBpsCredentialState'
const props = defineProps<{ state?: ExcelBPSCredentialState | null; accountId?: number }>()
const { t } = useI18n()
const now = useExcelCredentialClock()
const starting = ref(false)
const notice = ref('')
const noticeError = ref(false)
const status = computed(() => props.state ? excelCredentialStatus(props.state, now.value) : 'unknown')
const failed = computed(() => ['expired', 'revoked', 'auth_failed'].includes(status.value))
const canReauthorize = computed(() => status.value === 'pending' && typeof props.accountId === 'number' && props.accountId > 0)
const reauthorize = async () => {
  if (!canReauthorize.value || starting.value || props.accountId == null) return
  starting.value = true
  notice.value = ''
  noticeError.value = false
  try {
    await adminAPI.accounts.startExcelOAuthReauth(props.accountId)
    notice.value = t('admin.accounts.bpsCredentialState.reauthorizeStarted')
  } catch (error: unknown) {
    noticeError.value = true
    const message = error && typeof error === 'object' && 'message' in error ? String((error as { message?: unknown }).message || '') : ''
    notice.value = message || t('admin.accounts.bpsCredentialState.reauthorizeFailed')
  } finally {
    starting.value = false
  }
}
</script>
