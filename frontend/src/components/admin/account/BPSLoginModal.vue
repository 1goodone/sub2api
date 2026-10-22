<template>
  <Teleport to="body">
    <dialog
      v-if="show"
      ref="dialogRef"
      class="bps-login-dialog"
      data-testid="bps-login-dialog"
      aria-modal="true"
      :aria-labelledby="titleId"
      @cancel="onCancel"
    >
      <div class="modal-content w-full max-w-lg" data-testid="bps-login-modal" @click.stop>
        <div class="modal-header">
          <h3 :id="titleId" class="modal-title">{{ t('admin.accounts.bpsLogin.title') }}</h3>
          <button type="button" class="btn btn-secondary btn-sm" data-testid="bps-login-close" @click="close">
            {{ t('common.cancel') }}
          </button>
        </div>
        <div class="modal-body">
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
        <div class="modal-footer">
          <button type="button" class="btn btn-secondary" @click="close">{{ t('common.cancel') }}</button>
          <button type="button" class="btn btn-primary" :disabled="submitting || picked.size === 0 || proxyLines.length === 0" data-testid="bps-login-submit" @click="submit">
            {{ submitting ? t('admin.accounts.bpsLogin.submitting') : t('admin.accounts.bpsLogin.submit', { count: picked.size }) }}
          </button>
        </div>
      </div>
    </dialog>
  </Teleport>
</template>
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { BPSLoginAccountResult } from '@/api/admin/accounts'
import type { AccountListItem } from '@/types'
import { extractApiErrorMessage } from '@/utils/apiError'

const props = defineProps<{ show: boolean; accounts: AccountListItem[]; selectedIds: number[] }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const titleId = `bps-login-title-${Math.random().toString(36).slice(2, 8)}`
const dialogRef = ref<HTMLDialogElement | null>(null)
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

function present() {
  const dialog = dialogRef.value
  if (!dialog) return
  if (typeof dialog.showModal === 'function') {
    try {
      if (!dialog.open) dialog.showModal()
      return
    } catch {
      // showModal is missing or the element is already open; the open attribute still shows the form.
    }
  }
  dialog.setAttribute('open', '')
}

function dismiss() {
  const dialog = dialogRef.value
  if (dialog && dialog.open && typeof dialog.close === 'function') dialog.close()
}

watch(() => props.show, async (open) => {
  if (!open) {
    dismiss()
    return
  }
  query.value = ''
  proxiesText.value = ''
  results.value = []
  const visible = new Set(candidates.value.map(account => account.id))
  const selected = Array.isArray(props.selectedIds) ? props.selectedIds : []
  picked.value = new Set(selected.filter(id => visible.has(id)))
  await nextTick()
  present()
  dialogRef.value?.querySelector<HTMLElement>('[data-testid="bps-login-proxy"]')?.focus()
}, { immediate: true, flush: 'post' })

onBeforeUnmount(() => {
  dismiss()
})

function close() {
  if (submitting.value) return
  emit('close')
}

function onCancel(event: Event) {
  event.preventDefault()
  close()
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
<style>
.bps-login-dialog {
  position: fixed;
  inset: 0;
  z-index: 2147483646;
  width: 100%;
  height: 100%;
  max-width: none;
  max-height: none;
  margin: 0;
  padding: 16px;
  border: 0;
  background: rgba(2, 6, 23, 0.55);
  color: inherit;
  opacity: 1;
}
.bps-login-dialog[open] {
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 1;
}
.bps-login-dialog::backdrop {
  background: rgba(2, 6, 23, 0.55);
}
</style>
