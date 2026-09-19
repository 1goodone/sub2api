<template>
  <AppLayout>
    <div class="space-y-6 p-4 sm:p-6">
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 class="text-xl font-semibold text-gray-900 dark:text-gray-100">{{ t('admin.accounts.statePool.title') }}</h1>
          <p class="mt-2 max-w-4xl text-sm leading-6 text-gray-500 dark:text-gray-400">{{ t('admin.accounts.statePool.description') }}</p>
        </div>
        <RouterLink to="/admin/accounts" class="btn btn-secondary">{{ t('admin.accounts.statePool.manageAccounts') }}</RouterLink>
      </div>

      <section class="border-y border-gray-200 py-4 dark:border-dark-600">
        <label class="mb-2 block text-sm font-medium">{{ t('admin.accounts.statePool.globalProxy') }}</label>
        <div class="flex flex-wrap items-center gap-3">
          <div class="w-full sm:w-80"><ProxySelector v-model="proxyId" :proxies="proxies" :disabled="saving" /></div>
          <button class="btn btn-primary" :disabled="saving || !pool || (proxyId || 0) === pool.config.proxy_id" @click="saveProxy">{{ t('common.save') }}</button>
          <span v-if="pool?.config.proxy_name" class="text-sm text-gray-500">{{ pool.config.proxy_name }}</span>
        </div>
        <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('admin.accounts.statePool.proxyHint') }}</p>
        <p v-if="pool && !pool.config.available" role="status" class="mt-3 rounded-lg bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-900/20 dark:text-amber-300">
          {{ pool.config.issue === 'proxy_not_configured' ? t('admin.accounts.statePool.proxyMissing') : t('admin.accounts.statePool.proxyUnavailable') }}
        </p>
      </section>

      <div class="flex flex-wrap items-center gap-3">
        <input v-model="search" :placeholder="t('admin.accounts.statePool.search')" :aria-label="t('admin.accounts.statePool.search')" class="input w-full sm:w-64" />
        <span class="text-sm text-gray-500">{{ t('admin.accounts.statePool.summary', { valid: validCount, total: visibleRows.length }) }}</span>
        <button class="btn btn-secondary ml-auto" :aria-label="t('admin.accounts.statePool.reload')" :title="t('admin.accounts.statePool.reload')" :disabled="loading" @click="loadPool"><Icon name="refresh" size="sm" /></button>
      </div>
      <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">{{ error }}</p>
      <div v-if="!pool && loading" class="py-16 text-center text-gray-500">{{ t('common.loading') }}</div>
      <div v-else-if="!visibleRows.length" class="border-y border-dashed border-gray-300 py-12 text-center text-sm text-gray-500 dark:border-dark-600">
        {{ t('admin.accounts.statePool.empty') }}
      </div>
      <div v-else class="grid gap-5 md:grid-cols-2 xl:grid-cols-3">
        <article v-for="row in visibleRows" :key="row.account_id" class="min-w-0 rounded-lg border border-l-4 bg-white p-5 shadow-sm dark:bg-dark-800" :class="stateTone(row) === 'normal' ? 'border-gray-200 border-l-emerald-500 dark:border-dark-600 dark:border-l-emerald-500' : stateTone(row) === 'abnormal' ? 'border-gray-200 border-l-red-500 dark:border-dark-600 dark:border-l-red-500' : 'border-gray-200 border-l-gray-400 dark:border-dark-600 dark:border-l-gray-500'">
          <div class="flex flex-wrap items-start justify-between gap-2">
            <h2 class="break-all text-base font-semibold">#{{ row.account_id }} · {{ row.account_name }}</h2>
            <select :disabled="importing" :value="row.model" class="input w-full font-mono" :aria-label="t('admin.accounts.statePool.model')" @change="selectModel(row.account_id, ($event.target as HTMLSelectElement).value)">
              <option v-for="model in accountModels(row.account_id)" :key="model" :value="model">{{ model }}</option>
            </select>
            <span class="rounded px-2 py-1 text-xs font-medium" :class="stateTone(row) === 'normal' ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-300' : stateTone(row) === 'abnormal' ? 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300' : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'">{{ stateLabel(row) }}</span>
          </div>

          <p class="mt-1 text-xs text-gray-500">{{ row.plan || t('admin.accounts.statePool.unknownPlan') }} · {{ t('admin.accounts.statePool.expectedLength', { length: row.expected_length || '—' }) }}</p>

          <div class="mt-5 border-t border-gray-200 pt-3 dark:border-dark-600">
            <div class="flex items-center justify-between gap-2 text-xs">
              <span class="text-gray-500">{{ t('admin.accounts.statePool.remaining') }}</span>
              <span class="font-mono font-semibold tabular-nums">{{ remainingLabel(row) }}</span>
            </div>
            <div class="mt-2 h-1.5 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600"><div class="h-full rounded-full transition-all" :class="stateTone(row) === 'normal' ? 'bg-emerald-500' : stateTone(row) === 'abnormal' ? 'bg-red-500' : 'bg-gray-400'" :style="{ width: `${remainingPercent(row)}%` }" /></div>
            <p class="mt-2 text-xs text-gray-500">{{ t('admin.accounts.statePool.issuedAt') }} {{ formatTime(row.issued_at) }}</p>
            <p class="mt-1 text-xs text-gray-500">{{ t('admin.accounts.statePool.obtainedAt') }} {{ formatTime(row.obtained_at) }}</p>
            <p class="mt-2 text-xs text-gray-500">{{ t('admin.accounts.statePool.expiresAt') }} {{ formatTime(row.expires_at) }}</p>
          </div>

          <div class="mt-4 space-y-2 border-t border-gray-200 pt-3 text-xs dark:border-dark-600">
            <div class="flex justify-between gap-2"><span class="font-mono">X-Codex-Turn-State</span><span :class="stateTone(row) === 'normal' ? 'text-emerald-600 dark:text-emerald-300' : stateTone(row) === 'abnormal' ? 'text-red-600 dark:text-red-300' : 'text-gray-500'">{{ row.state_length || '—' }}</span></div>
            <p class="font-mono text-gray-400">•••• •••• •••• ••••</p>
            <p class="text-gray-500">{{ t('admin.accounts.statePool.rawHint') }}</p>
          </div>
          <div class="mt-4 min-h-14 space-y-1 text-xs text-gray-500">
            <p>{{ probeLabel(row) }} · {{ modeLabel(row.mode) }}<span v-if="row.attempts > 0"> · #{{ row.attempts }}</span></p>
            <p>{{ t('admin.accounts.statePool.nextProbe') }} {{ formatTime(row.next_probe_at) }}</p>
            <p>{{ t('admin.accounts.statePool.lastAttempt') }} {{ formatTime(row.last_attempt_at) }}</p>
            <p v-if="row.reason" class="text-amber-600 dark:text-amber-400">{{ reasonLabel(row.reason) }}</p>
          </div>
          <div class="mt-4 flex flex-wrap justify-between gap-2">
            <button class="btn btn-secondary btn-sm" :aria-label="t('admin.accounts.statePool.copy')" :title="t('admin.accounts.statePool.copy')" :disabled="!isValid(row) || copying === rowKey(row)" @click="copyState(row)"><Icon name="copy" size="sm" /></button>
            <button class="btn btn-primary btn-sm" :disabled="!row.can_refresh || refreshing.has(rowKey(row))" @click="queueRefresh(row)"><Icon name="refresh" size="sm" />{{ t('admin.accounts.statePool.refresh') }}</button>
          </div>
          <button class="btn btn-secondary btn-sm mt-3" :disabled="!row.can_refresh || importing" @click="openImport(row)"><Icon name="clipboard" size="sm" />{{ t('admin.accounts.statePool.paste') }}</button>
          <div v-if="importKey === rowKey(row)" class="mt-3 space-y-2">
            <textarea v-model="pastedState" :disabled="importing" class="input min-h-24 font-mono text-xs" autocomplete="off" spellcheck="false" :aria-label="t('admin.accounts.statePool.paste')" maxlength="4096" />
            <p class="text-xs">{{ t('admin.accounts.statePool.inputLength', { length: pastedState.trim().length }) }}</p>
            <p v-if="importError" role="alert" class="text-sm text-red-600">{{ importError }}</p>
            <button class="btn btn-primary btn-sm" :disabled="importing || !pastedState.trim()" @click="submitImport(row)">{{ t('admin.accounts.statePool.validateReplace') }}</button>
            <button class="btn btn-secondary btn-sm ml-2" :disabled="importing" @click="closeImport">{{ t('common.cancel') }}</button>
          </div>
          <button class="mt-4 text-sm text-primary-600" @click="toggleLogs(row)">{{ t('admin.accounts.statePool.logs') }}</button>
          <div v-if="logsOpen.has(row.account_id)" class="mt-2 max-h-80 space-y-3 overflow-auto rounded-lg bg-gray-50 p-3 text-xs dark:bg-dark-900/60">
            <p v-if="logErrors[rowKey(row)]" role="alert">{{ t('admin.accounts.statePool.loadLogsFailed') }}</p>
            <p v-else-if="!events[rowKey(row)]?.length">{{ t('admin.accounts.statePool.noLogs') }}</p>
            <div v-for="event in events[rowKey(row)] || []" :key="event.id" class="border-b border-gray-200 pb-2 dark:border-dark-600">
              <p>{{ formatTime(event.created_at) }} · {{ eventLabel(event.kind) }} · {{ event.source }} · #{{ event.attempt }}</p>
              <p>{{ reasonLabel(event.reason) }}</p>
              <p>{{ t('admin.accounts.statePool.sent') }} {{ event.sent_length }} → {{ t('admin.accounts.statePool.returned') }} {{ event.returned_length }} · HTTP {{ event.http_status || '—' }} · {{ event.duration_ms }} ms</p>
              <p v-if="event.proxy_id">{{ event.proxy_name }} #{{ event.proxy_id }} · {{ event.proxy_address }}</p>
              <p v-if="event.proxy_id">{{ t('admin.accounts.statePool.referenceIP') }}: {{ event.reference_ip || t('admin.accounts.statePool.ipUnknown') }} · {{ event.ip_status }}</p>
            </div>
            <button v-if="hasMore[rowKey(row)]" class="text-primary-600" @click="loadEvents(row, true)">{{ t('admin.accounts.statePool.moreLogs') }}</button>
          </div>
        </article>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import ProxySelector from '@/components/common/ProxySelector.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { useClipboard } from '@/composables/useClipboard'
import { getAll as getProxies } from '@/api/admin/proxies'
import { getStatePool, saveStatePoolProxy, refreshState, getStateValue, getStateEvents, importState, getImportJob, type StateEvent, type StatePoolRow, type StatePoolResponse } from '@/api/admin/codexStatePool'
import type { Proxy } from '@/types'

const { t } = useI18n()
const app = useAppStore()
const { copyToClipboard } = useClipboard()
const pool = ref<StatePoolResponse | null>(null)
const proxies = ref<Proxy[]>([])
const proxyId = ref<number | null>(null)
const selectedModels = ref<Record<number, string>>({})
const events = ref<Record<string, StateEvent[]>>({})
const logsOpen = ref(new Set<number>())
const hasMore = ref<Record<string, boolean>>({})
const logErrors = ref<Record<string, boolean>>({})
const importKey = ref('')
const pastedState = ref('')
const importing = ref(false)
const importError = ref('')
const importStatus = ref('')
const search = ref('')
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const copying = ref('')
const refreshing = ref(new Set<string>())
const now = ref(Date.now())
let serverOffset = 0
let controller: AbortController | null = null
let poll: ReturnType<typeof setInterval> | undefined
let clock: ReturnType<typeof setInterval> | undefined
let disposed = false
let configLoaded = false
const accountModels = (id: number) => (pool.value?.items || []).filter(row => row.account_id === id).map(row => row.model)
const visibleRows = computed(() => {
  const groups = new Map<number, StatePoolRow[]>()
  for (const row of pool.value?.items || []) { const rows = groups.get(row.account_id) || []; rows.push(row); groups.set(row.account_id, rows) }
  return [...groups.values()].map(rows => rows.find(row => row.model === selectedModels.value[row.account_id]) || rows.find(row => row.model === 'gpt-6-astra') || rows[0]!).filter(row => `${row.account_id} ${row.account_name} ${row.plan}`.toLowerCase().includes(search.value.trim().toLowerCase()))
})
function selectModel(id: number, model: string) { selectedModels.value[id] = model; closeImport(); const row = visibleRows.value.find(row => row.account_id === id); if (row && logsOpen.value.has(id)) void loadEvents(row) }
function modeLabel(mode?: string) { return t(`admin.accounts.statePool.mode_${mode || 'idle'}`) }
function eventLabel(kind: string) { return t(`admin.accounts.statePool.event_${kind}`) }
async function loadEvents(row: StatePoolRow, more = false) {
  const key = rowKey(row)
  try {
    const previous = events.value[key] || []
    const result = await getStateEvents(row, more ? previous[previous.length - 1]?.id || 0 : 0)
    if (disposed) return
    const merged = new Map([...previous, ...result].map(event => [event.id, event]))
    events.value[key] = [...merged.values()].filter(event => Date.parse(event.created_at) > now.value - 7 * 86400000).sort((a, b) => b.id - a.id)
    if (more || !previous.length) hasMore.value[key] = result.length === 50
    logErrors.value[key] = false
  } catch { logErrors.value[key] = true }
}
function toggleLogs(row: StatePoolRow) { if (logsOpen.value.has(row.account_id)) logsOpen.value.delete(row.account_id); else { logsOpen.value.add(row.account_id); void loadEvents(row) } }
function closeImport() { importKey.value = ''; pastedState.value = ''; importError.value = ''; importStatus.value = '' }
function openImport(row: StatePoolRow) { closeImport(); importKey.value = rowKey(row) }
async function submitImport(row: StatePoolRow) {
  importing.value = true; importError.value = ''
  try {
    const job = await importState(row, pastedState.value.trim())
    closeImport(); importStatus.value = job.status
    app.showSuccess(t('admin.accounts.statePool.importQueued'))
    void watchImportJob(row, job.id)
    await loadPool()
  }
  catch (error: any) { const reason = error?.message || error?.detail || error?.response?.data?.message || error?.response?.data?.detail || t('admin.accounts.statePool.refreshFailed'); importError.value = reasonLabel(reason) }
  finally { importing.value = false; if (logsOpen.value.has(row.account_id)) void loadEvents(row) }
}
async function watchImportJob(row: StatePoolRow, jobId: string) {
  for (let attempt = 0; attempt < 120 && !disposed; attempt += 1) {
    await new Promise(resolve => window.setTimeout(resolve, 500))
    try {
      const job = await getImportJob(row, jobId)
      if (job.status === 'succeeded') { app.showSuccess(t('admin.accounts.statePool.imported')); await loadPool(); return }
      if (job.status === 'failed') { app.showError(reasonLabel(job.reason || 'probe_failed')); await loadPool(); return }
    } catch { return }
  }
}
const validCount = computed(() => visibleRows.value.filter(isValid).length)
const rowKey = (row: StatePoolRow) => `${row.account_id}:${row.model}`
const remaining = (row: StatePoolRow) => row.expires_at ? Math.max(0, Date.parse(row.expires_at) - now.value) : 0
const isValid = (row: StatePoolRow) => row.state_status === 'valid' && remaining(row) > 0
function stateTone(row: StatePoolRow): 'normal' | 'abnormal' | 'unknown' {
  if (row.state_status === 'disabled' || row.state_status === 'unknown_plan') return 'unknown'
  if (row.expected_length > 0 && row.state_length > 0) {
    return row.state_length === row.expected_length && isValid(row) ? 'normal' : 'abnormal'
  }
  if (row.state_status === 'invalid' || row.state_status === 'expired') return 'abnormal'
  return 'unknown'
}
function remainingPercent(row: StatePoolRow) {
  if (!isValid(row) || !row.obtained_at || !row.expires_at) return 0
  const duration = Date.parse(row.expires_at) - Date.parse(row.obtained_at)
  return duration > 0 ? Math.min(100, remaining(row) / duration * 100) : 0
}
function remainingLabel(row: StatePoolRow) {
  if (!row.expires_at) return '—'
  const seconds = Math.floor(remaining(row) / 1000)
  if (!seconds) return t('admin.accounts.statePool.expired')
  return `${Math.floor(seconds / 60).toString().padStart(2, '0')}:${(seconds % 60).toString().padStart(2, '0')}`
}
function formatTime(value?: string) { return value ? new Date(value).toLocaleString() : '—' }
function stateLabel(row: StatePoolRow) {
  if (row.state_status === 'expired' || (row.state_status === 'valid' && !isValid(row))) return t('admin.accounts.statePool.expired')
  if (stateTone(row) === 'normal') return t('admin.accounts.statePool.valid', { length: row.state_length })
  if (stateTone(row) === 'abnormal') return t('admin.accounts.statePool.invalid')
  if (row.state_status === 'unknown_plan') return t('admin.accounts.statePool.unknownPlan')
  if (row.state_status === 'disabled') return t('admin.accounts.statePool.disabled')
  if (row.state_status === 'invalid') return t('admin.accounts.statePool.invalid')
  return t('admin.accounts.statePool.missing')
}
function probeLabel(row: StatePoolRow) {
  if (row.probe_status === 'running') return t('admin.accounts.statePool.running')
  if (row.probe_status === 'queued') return t('admin.accounts.statePool.queued')
  if (row.probe_status === 'stopped') return t('admin.accounts.statePool.stopped')
  if (row.probe_status === 'blocked') return t('admin.accounts.statePool.blocked')
  return t('admin.accounts.statePool.idle')
}
function reasonLabel(reason: string) {
  if (reason === 'response_created_gpt_5_6_luna') return t('admin.accounts.statePool.downgraded')
  if (reason === 'server_is_overloaded') return t('admin.accounts.statePool.overloaded')
  if (reason.startsWith('state_length_') || reason === 'unexpected_state_length') return t('admin.accounts.statePool.abnormalLength')
  const known: Record<string, string> = { replaced: 'replaced', same_state: 'sameState', not_newer: 'notNewer', account_busy: 'accountBusy', state_not_accepted: 'stateRejected', validation_not_completed: 'validationIncomplete', invalid_state_format: 'invalidFormat', future_state_timestamp: 'futureTimestamp', expired_state: 'expired', account_changed: 'accountChanged', concurrent_update: 'concurrentUpdate', invalid_encrypted_content: 'encryptedError', candidate_already_changed: 'concurrentUpdate' }
  if (known[reason]) return t(`admin.accounts.statePool.${known[reason]}`)
  return reason || '—'
}
async function loadPool() {
  if (loading.value || disposed) return
  loading.value = true
  controller = new AbortController()
  try {
    const result = await getStatePool(controller.signal)
    if (disposed) return
    pool.value = result
    serverOffset = Date.parse(result.server_time) - Date.now()
    now.value = Date.now() + serverOffset
    if (!configLoaded) { proxyId.value = result.config.proxy_id || null; configLoaded = true }
    error.value = ''
    for (const row of visibleRows.value) { if (logsOpen.value.has(row.account_id)) void loadEvents(row) }
  } catch (err: any) {
    if (!disposed && err?.code !== 'ERR_CANCELED') error.value = t('admin.accounts.statePool.loadFailed')
  } finally { loading.value = false }
}
async function saveProxy() {
  saving.value = true
  try { await saveStatePoolProxy(proxyId.value || 0); app.showSuccess(t('admin.accounts.statePool.saved')); await loadPool() }
  catch { app.showError(t('admin.accounts.statePool.saveFailed')) }
  finally { saving.value = false }
}
async function queueRefresh(row: StatePoolRow) {
  refreshing.value.add(rowKey(row))
  try { await refreshState(row); app.showSuccess(t('admin.accounts.statePool.refreshQueued')); await loadPool() }
  catch { app.showError(t('admin.accounts.statePool.refreshFailed')) }
  finally { refreshing.value.delete(rowKey(row)) }
}
async function copyState(row: StatePoolRow) {
  copying.value = rowKey(row)
  try { await copyToClipboard(await getStateValue(row), t('admin.accounts.statePool.copied')) }
  catch { app.showError(t('admin.accounts.statePool.copyFailed')) }
  finally { copying.value = '' }
}
onMounted(() => {
  void loadPool()
  void getProxies().then(value => { if (!disposed) proxies.value = value }).catch(() => app.showError(t('admin.accounts.statePool.loadProxiesFailed')))
  clock = setInterval(() => { now.value = Date.now() + serverOffset }, 1000)
  poll = setInterval(() => { if (!document.hidden) void loadPool() }, 5000)
})
onUnmounted(() => { disposed = true; closeImport(); controller?.abort(); clearInterval(poll); clearInterval(clock) })
</script>
