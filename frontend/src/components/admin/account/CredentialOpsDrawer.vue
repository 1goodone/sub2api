<template>
  <div v-if="account" class="fixed inset-0 z-50 flex justify-end bg-black/40" @click.self="emit('close')">
    <aside class="flex h-full w-full max-w-md flex-col bg-white shadow-xl dark:bg-dark-800" role="dialog" aria-modal="true" data-testid="credential-ops-drawer">
      <div class="flex items-center justify-between border-b border-gray-100 px-4 py-3 dark:border-dark-700">
        <div class="min-w-0">
          <h2 class="truncate text-base font-semibold text-gray-900 dark:text-gray-100">{{ t('tokenGuardV2.title') }}</h2>
          <p class="truncate text-xs text-gray-500">{{ account.name }} · #{{ account.id }}</p>
        </div>
        <button type="button" class="text-gray-400 hover:text-gray-600" @click="emit('close')">×</button>
      </div>
      <div class="flex-1 space-y-4 overflow-y-auto px-4 py-4">
        <p v-if="loading" class="text-sm text-gray-500">{{ t('common.loading') }}</p>
        <p v-else-if="error" class="text-sm text-red-600">{{ error }}</p>
        <template v-else-if="item">
          <div class="grid grid-cols-2 gap-3 text-sm">
            <div><div class="text-xs text-gray-400">{{ t('tokenGuardV2.configuration') }}</div><div>{{ item.login_config?.configured ? t('tokenGuardV2.configured') : t('tokenGuardV2.incomplete') }}</div></div>
            <div><div class="text-xs text-gray-400">{{ t('tokenGuardV2.inspection') }}</div><div>{{ item.probe_state || '-' }}</div></div>
            <div><div class="text-xs text-gray-400">{{ t('tokenGuardV2.failureStreak') }}</div><div>{{ item.fail_streak }}</div></div>
            <div><div class="text-xs text-gray-400">{{ t('tokenGuardV2.lastInspection') }}</div><div>{{ date(item.last_probe_at) }}</div></div>
            <div class="col-span-2"><div class="text-xs text-gray-400">{{ t('tokenGuardV2.lastRelogin') }}</div><div>{{ date(item.last_reauth_at) }}<span v-if="item.latest_task"> · {{ item.latest_task.status }}</span></div></div>
          </div>
          <label class="flex items-center justify-between text-sm"><span>{{ t('tokenGuardV2.autoInspect') }}</span><input type="checkbox" role="switch" :checked="item.enabled" :disabled="busy" @change="toggle('enabled', $event)" /></label>
          <label class="flex items-center justify-between text-sm"><span>{{ t('tokenGuardV2.autoRelogin') }}</span><input type="checkbox" role="switch" :checked="item.auto_relogin_enabled" :disabled="busy" @change="toggle('auto_relogin_enabled', $event)" /></label>
          <p v-if="notice" class="text-xs text-emerald-600">{{ notice }}</p>
          <div class="flex flex-wrap gap-2">
            <button type="button" class="btn btn-secondary" :disabled="busy" @click="run('probe')">{{ t('tokenGuardV2.inspectNow') }}</button>
            <button type="button" class="btn btn-secondary" :disabled="busy" @click="run('relogin')">{{ t('tokenGuardV2.reloginNow') }}</button>
            <OpenAITOTPDialog :account-id="item.account_id" :account-name="item.account_name" :configured="!!(item.login_config?.password_configured && item.login_config?.totp_configured)" />
          </div>
        </template>
        <p v-else class="text-sm text-gray-500">{{ t('admin.accounts.credentialOps.notEnrolled') }}</p>
        <p class="text-xs text-gray-400">{{ t('admin.accounts.credentialOps.hint') }}</p>
      </div>
      <div class="border-t border-gray-100 px-4 py-3 dark:border-dark-700">
        <router-link class="text-sm text-primary-600 hover:underline" :to="pageLink">{{ t('admin.accounts.credentialOps.openPage') }}</router-link>
      </div>
    </aside>
  </div>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import OpenAITOTPDialog from '@/components/account/OpenAITOTPDialog.vue'
import { listTokenGuardV2Accounts, probeTokenGuardV2Account, reloginTokenGuardV2Account, updateTokenGuardV2Switches, type TokenGuardV2Account } from '@/api/admin/accountTokenGuardV2'
import type { Account } from '@/types'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'

const props = defineProps<{ account: Account | null }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const item = ref<TokenGuardV2Account | null>(null)
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const pageLink = computed(() => ({ path: '/admin/token-guard-v2', query: props.account ? { account: String(props.account.id) } : {} }))

watch(() => props.account?.id, (id) => { if (id) void load() }, { immediate: true })

function date(value?: string) {
  return value ? formatDateTime(value) : '-'
}
async function load() {
  if (!props.account) return
  loading.value = true
  error.value = ''
  notice.value = ''
  try {
    const status = await listTokenGuardV2Accounts()
    item.value = status.accounts.find(row => row.account_id === props.account?.id) || null
  } catch (value) {
    error.value = extractApiErrorMessage(value, t('admin.accounts.credentialOps.loadFailed'))
  } finally {
    loading.value = false
  }
}
async function run(action: 'probe' | 'relogin') {
  if (!item.value || busy.value) return
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    if (action === 'probe') await probeTokenGuardV2Account(item.value.account_id)
    else {
      const result = await reloginTokenGuardV2Account(item.value.account_id)
      const refreshed = !!result && typeof result === 'object' && 'refreshed' in result && result.refreshed
      notice.value = t(refreshed ? 'tokenGuardV2.tokenRefreshed' : 'tokenGuardV2.reloginQueued', { account: item.value.account_name })
    }
    if (action === 'probe') notice.value = t('tokenGuardV2.inspectionQueued', { account: item.value.account_name })
    await load()
  } catch (value) {
    error.value = extractApiErrorMessage(value, t('admin.accounts.credentialOps.actionFailed'))
  } finally {
    busy.value = false
  }
}
async function toggle(field: 'enabled' | 'auto_relogin_enabled', event: Event) {
  const input = event.target as HTMLInputElement
  if (!item.value || busy.value) { input.checked = !!item.value?.[field]; return }
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    const saved = await updateTokenGuardV2Switches(item.value.account_id, { [field]: !item.value[field] })
    Object.assign(item.value, saved)
    notice.value = t('tokenGuardV2.switchSaved')
  } catch (value) {
    error.value = extractApiErrorMessage(value, t('admin.accounts.credentialOps.actionFailed'))
  } finally {
    if (item.value) input.checked = item.value[field]
    busy.value = false
  }
}
</script>
