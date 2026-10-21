<template>
  <div v-if="show" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" @click.self="close">
    <div class="flex max-h-[85vh] w-full max-w-lg flex-col rounded-xl bg-white shadow-xl dark:bg-dark-800" role="dialog" aria-modal="true" data-testid="bps-login-modal">
      <div class="flex items-center justify-between border-b border-gray-100 px-4 py-3 dark:border-dark-700">
        <h2 class="text-base font-semibold text-gray-900 dark:text-gray-100">{{ t('admin.accounts.bpsLogin.title') }}</h2>
        <button type="button" class="text-gray-400 hover:text-gray-600" @click="close">×</button>
      </div>
      <div class="space-y-3 overflow-y-auto px-4 py-3">
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.bpsLogin.hint') }}</p>
        <input
          v-model="query"
          type="search"
          class="input w-full"
          :placeholder="t('admin.accounts.bpsLogin.search')"
        />
        <div class="max-h-52 overflow-y-auto rounded-lg border border-gray-200 dark:border-dark-600">
          <label v-for="account in filtered" :key="account.id" class="flex items-center gap-2 border-b border-gray-100 px-3 py-2 text-sm last:border-b-0 dark:border-dark-700">
            <input type="checkbox" class="rounded border-gray-300 text-primary-600" :checked="picked.has(account.id)" @change="toggle(account.id)" />
            <span class="min-w-0 flex-1 truncate text-gray-800 dark:text-gray-100">{{ account.name || ('#' + account.id) }}</span>
            <span class="text-xs text-gray-400">#{{ account.id }}</span>
          </label>
          <p v-if="filtered.length === 0" class="px-3 py-6 text-center text-sm text-gray-400">{{ t('admin.accounts.bpsLogin.empty') }}</p>
        </div>
        <label class="block text-sm text-gray-700 dark:text-gray-200">
          {{ t('admin.accounts.bpsLogin.proxy') }}
          <textarea
            v-model="proxiesText"
            rows="4"
            autocomplete="off"
            spellcheck="false"
            class="input mt-1 w-full font-mono"
            :placeholder="t('admin.accounts.bpsLogin.proxyPlaceholder')"
            data-testid="bps-login-proxy"
          />
        </label>
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.bpsLogin.proxyHint') }}</p>
        <ul v-if="results.length" class="space-y-1 text-xs">
          <li v-for="item in results" :key="item.account_id" :class="item.error ? 'text-red-600 dark:text-red-400' : 'text-emerald-600 dark:text-emerald-400'">
            #{{ item.account_id }} {{ item.error || t('admin.accounts.bpsLogin.started', { id: item.task_id }) }}
          </li>
        </ul>
      </div>
      <div class="flex justify-end gap-2 border-t border-gray-100 px-4 py-3 dark:border-dark-700">
        <button type="button" class="btn btn-secondary" @click="close">{{ t('common.cancel') }}</button>
        <button type="button" class="btn btn-primary" :disabled="submitting || picked.size === 0 || proxyLines.length === 0" data-testid="bps-login-submit" @click="submit">
          {{ submitting ? t('admin.accounts.bpsLogin.submitting') : t('admin.accounts.bpsLogin.submit', { count: picked.size }) }}
        </button>
      </div>
    </div>
  </div>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { BPSLoginAccountResult } from '@/api/admin/accounts'
import type { AccountListItem } from '@/types'
import { extractApiErrorMessage } from '@/utils/apiError'

const props = defineProps<{ show: boolean; accounts: AccountListItem[]; selectedIds: number[] }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const query = ref('')
const proxiesText = ref('')
const picked = ref(new Set<number>())
const submitting = ref(false)
const results = ref<BPSLoginAccountResult[]>([])

function bpsEligible(account: AccountListItem) {
  if (account.platform !== 'openai' || account.type !== 'oauth' || account.parent_account_id != null) return false
  const credential = (key: string) => {
    const value = account.credentials?.[key]
    return typeof value === 'string' ? value.trim().toLowerCase() : ''
  }
  if (credential('plan_type') === 'free') return false
  const personal = (mode: string) => mode === 'personalaccesstoken' || mode === 'personal_access_token'
  const auth = credential('auth_mode')
  return auth !== 'agentidentity' && !personal(auth) && !personal(credential('openai_auth_mode'))
}
const proxyLines = computed(() => proxiesText.value.split(/\r?\n/).map(line => line.trim()).filter(Boolean))
const candidates = computed(() => props.accounts.filter(bpsEligible))
const filtered = computed(() => {
  const text = query.value.trim().toLowerCase()
  if (!text) return candidates.value
  return candidates.value.filter(account => account.name.toLowerCase().includes(text) || String(account.id).includes(text))
})

watch(() => props.show, (open) => {
  if (!open) return
  query.value = ''
  proxiesText.value = ''
  results.value = []
  const visible = new Set(candidates.value.map(account => account.id))
  picked.value = new Set(props.selectedIds.filter(id => visible.has(id)))
})

function toggle(id: number) {
  const next = new Set(picked.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  picked.value = next
}
function close() {
  if (!submitting.value) emit('close')
}
async function submit() {
  if (submitting.value || picked.value.size === 0 || proxyLines.value.length === 0) return
  submitting.value = true
  results.value = []
  try {
    const data = await adminAPI.accounts.startBPSLogin([...picked.value], proxyLines.value)
    results.value = data.results || []
    proxiesText.value = ''
  } catch (error) {
    results.value = [{ account_id: 0, error: extractApiErrorMessage(error, t('admin.accounts.bpsLogin.failed')) }]
  } finally {
    submitting.value = false
  }
}
</script>
