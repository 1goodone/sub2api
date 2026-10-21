<template>
  <div class="text-xs" :data-testid="'login-column-' + kind">
    <span v-if="!applicable" class="text-gray-300 dark:text-dark-500">—</span>
    <span v-else-if="mode === 'loading'" class="text-gray-400">…</span>
    <button v-else-if="mode === 'add' || mode === 'relogin'" type="button" class="rounded border px-1.5 py-0.5 font-medium leading-4 disabled:opacity-50" :class="mode === 'add' ? 'border-gray-300 text-gray-600 hover:bg-gray-50 dark:border-dark-500 dark:text-gray-300' : 'border-amber-300 text-amber-700 hover:bg-amber-50 dark:border-amber-700 dark:text-amber-300'" :disabled="busy" @click="act">
      {{ busy ? t('admin.accounts.loginColumns.working') : t(mode === 'add' ? 'admin.accounts.loginColumns.add' : 'admin.accounts.loginColumns.relogin') }}
    </button>
    <span v-else :class="mode === 'ok' ? 'text-emerald-600 dark:text-emerald-400' : 'text-gray-500'">{{ label }}</span>
    <p v-if="error" class="mt-1 max-w-[8rem] text-red-600 dark:text-red-400">{{ error }}</p>
  </div>
</template>
<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { reloginTokenGuardV2Account, type TokenGuardV2Account } from '@/api/admin/accountTokenGuardV2'
import { loginSIWCWithSavedCredentials, type SIWCStatus } from '@/api/admin/siwc'
import type { AccountListItem } from '@/types'
import { extractApiErrorMessage } from '@/utils/apiError'
import { excelCredentialStatus } from '@/utils/excelBpsCredentialState'

const props = defineProps<{ kind: 'credential' | 'bps' | 'siwc'; account: AccountListItem; guard?: TokenGuardV2Account | null; siwc?: SIWCStatus | null; siwcLoading?: boolean }>()
const emit = defineEmits<{ add: []; changed: [] }>()
const { t } = useI18n()
const busy = ref(false)
const error = ref('')
const applicable = computed(() => props.account.platform === 'openai' && props.account.type === 'oauth' && props.account.parent_account_id == null)

const mode = computed(() => {
  if (!applicable.value) return 'skip'
  if (props.kind === 'credential') return credentialMode(props.guard)
  if (props.kind === 'bps') return bpsMode()
  if (props.siwcLoading && !props.siwc) return 'loading'
  return siwcMode(props.siwc)
})
const label = computed(() => t(`admin.accounts.loginColumns.${mode.value === 'progress' ? progressKey() : 'ok'}`))

const activeTask = (status?: string) => status === 'queued' || status === 'running' || status === 'callback_processing'
function credentialMode(guard?: TokenGuardV2Account | null) {
  if (!guard?.login_config?.configured) return 'add'
  const task = guard.latest_task?.status
  if (activeTask(task)) return 'progress'
  if (guard.blocked_reason || guard.probe_state === 'auth' || guard.probe_state === 'transient' || task === 'failed') return 'relogin'
  return 'ok'
}
function bpsMode() {
  const state = props.account.excel_bps_credential_state
  if (!state) return 'add'
  const status = excelCredentialStatus(state, Date.now())
  if (status === 'not_expired') return 'ok'
  if (status === 'pending') return 'progress'
  if (status === 'expired' || status === 'revoked' || status === 'auth_failed') return 'relogin'
  return 'add'
}
function siwcMode(status?: SIWCStatus | null) {
  if (!status?.bound) return 'add'
  if (activeTask(status.task?.status)) return 'progress'
  if (status.refreshable === false || status.task?.status === 'failed' || status.task?.error) return 'relogin'
  return 'ok'
}
function progressKey() {
  return props.kind === 'bps' ? 'authorizing' : 'running'
}
async function act() {
  error.value = ''
  if (mode.value === 'add' || props.kind === 'bps') { emit('add'); return }
  busy.value = true
  try {
    if (props.kind === 'credential') await reloginTokenGuardV2Account(props.account.id)
    else await loginSIWCWithSavedCredentials(props.account.id)
    emit('changed')
  } catch (cause) {
    error.value = extractApiErrorMessage(cause, t('admin.accounts.loginColumns.failed'))
  } finally {
    busy.value = false
  }
}
</script>
