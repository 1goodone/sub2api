<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.bpsLogin.title')"
    width="normal"
    :z-index="100000030"
    :close-on-click-outside="false"
    @close="close"
  >
    <div data-testid="bps-login-modal">
      <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.bpsLogin.hint') }}</p>
      <input
        v-model="query"
        type="search"
        class="input mt-3 w-full"
        :placeholder="t('admin.accounts.bpsLogin.search')"
      />
      <div class="mt-3 max-h-52 overflow-y-auto rounded-lg border border-gray-200 dark:border-dark-600">
        <label v-for="account in filtered" :key="account.id" class="flex items-center gap-2 border-b border-gray-100 px-3 py-2 text-sm last:border-b-0 dark:border-dark-700">
          <input type="checkbox" class="rounded border-gray-300 text-primary-600" :checked="picked.has(account.id)" @change="toggle(account.id)" />
          <span class="min-w-0 flex-1 truncate text-gray-800 dark:text-gray-100">{{ account.name || ('#' + account.id) }}</span>
          <span class="text-xs text-gray-400">#{{ account.id }}</span>
        </label>
        <p v-if="filtered.length === 0" class="px-3 py-6 text-center text-sm text-gray-400">{{ t('admin.accounts.bpsLogin.empty') }}</p>
      </div>
      <label class="mt-3 block text-sm text-gray-700 dark:text-gray-200">
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
      <p class="mt-3 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.bpsLogin.proxyHint') }}</p>
      <ul v-if="results.length" class="mt-3 space-y-1 text-xs">
        <li v-for="item in results" :key="item.account_id" :class="item.error ? 'text-red-600 dark:text-red-400' : 'text-emerald-600 dark:text-emerald-400'">
          #{{ item.account_id }} {{ item.error || t('admin.accounts.bpsLogin.started', { id: item.task_id }) }}
        </li>
      </ul>
    </div>
    <template #footer>
      <button type="button" class="btn btn-secondary" @click="close">{{ t('common.cancel') }}</button>
      <button type="button" class="btn btn-primary" :disabled="submitting || picked.size === 0 || proxyLines.length === 0" data-testid="bps-login-submit" @click="submit">
        {{ submitting ? t('admin.accounts.bpsLogin.submitting') : t('admin.accounts.bpsLogin.submit', { count: picked.size }) }}
      </button>
    </template>
  </BaseDialog>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { BPSLoginAccountResult } from '@/api/admin/accounts'
import type { AccountListItem } from '@/types'
import { extractApiErrorMessage } from '@/utils/apiError'
import BaseDialog from '@/components/common/BaseDialog.vue'

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
const candidates = computed(() => (Array.isArray(props.accounts) ? props.accounts : []).filter(bpsEligible))
const filtered = computed(() => {
  const text = query.value.trim().toLowerCase()
  if (!text) return candidates.value
  return candidates.value.filter(account => (account.name || '').toLowerCase().includes(text) || String(account.id).includes(text))
})

watch(() => props.show, (open) => {
  if (!open) return
  query.value = ''
  proxiesText.value = ''
  results.value = []
  const visible = new Set(candidates.value.map(account => account.id))
  const selected = Array.isArray(props.selectedIds) ? props.selectedIds : []
  picked.value = new Set(selected.filter(id => visible.has(id)))
}, { immediate: true })

function close() {
  if (submitting.value) return
  emit('close')
}

function toggle(id: number) {
  const next = new Set(picked.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  picked.value = next
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
